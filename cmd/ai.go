/*
Copyright © 2025 Masayuki Yamai <twsnmp@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaesslerAG/jsonpath"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/weaviate/weaviate-go-client/v4/weaviate"
	"github.com/weaviate/weaviate-go-client/v4/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"
	"go.etcd.io/bbolt"
)

var weaviateURL = ""
var ollamaURL = ""
var text2vecModel = ""
var generativeModel = ""
var aiClass = ""
var aiLimit = 2

// aiCmd represents the ai command
var aiCmd = &cobra.Command{
	Use:   "ai [list|add|delete]",
	Short: "ai command",
	Long:  `manage ai config and export or ask ai`,
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
			return err
		}
		switch args[0] {
		case "list":
		case "add":
		case "delete":
		case "talk":
		default:
			return fmt.Errorf("invalid subcommand specified: %s", args[0])
		}
		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			log.Fatalln("you have to specify subcommand.")
		}
		switch args[0] {
		case "list":
			aiList()
		case "add":
			aiAdd()
		case "delete":
			aiDelete()
		case "talk":
			setupFilter(args[1:])
			aiTalk()
		}
	},
}

func init() {
	rootCmd.AddCommand(aiCmd)
	aiCmd.Flags().StringVar(&weaviateURL, "weaviate", "http://localhost:8080", "Weaviate URL")
	aiCmd.Flags().StringVar(&ollamaURL, "ollama", "http://host.docker.internal:11434", "Ollama URL")
	aiCmd.Flags().StringVar(&text2vecModel, "text2vec", "nomic-embed-text", "Text to vector model")
	aiCmd.Flags().StringVar(&generativeModel, "generative", "llama3.2", "Generative Model")
	aiCmd.Flags().StringVar(&aiClass, "aiClass", "", "Weaviate class name")
	aiCmd.Flags().IntVar(&aiLimit, "aiLimit", 2, "Limit value")
}

func aiList() {
	if weaviateURL == "" {
		log.Fatalln("you have to specify weaviate url")
	}
	client, err := getWeaviateClient()
	if err != nil {
		log.Fatalln(err)
	}
	schema, err := client.Schema().Getter().Do(context.Background())
	if err != nil {
		log.Fatalln(err)
	}
	hit := 0
	fmt.Println("Class\tOllama\ttext2vec\tgenerative")
	for _, c := range schema.Classes {
		oa, err := jsonpath.Get(`$["generative-ollama"].apiEndpoint`, c.ModuleConfig)
		if err != nil {
			continue
		}
		va, err := jsonpath.Get(`$["text2vec-ollama"].apiEndpoint`, c.ModuleConfig)
		if err != nil {
			continue
		}
		if va != oa {
			continue
		}
		o, ok := oa.(string)
		if !ok {
			continue
		}
		gm, err := jsonpath.Get(`$["generative-ollama"].model`, c.ModuleConfig)
		if err != nil {
			continue
		}
		vm, err := jsonpath.Get(`$["text2vec-ollama"].model`, c.ModuleConfig)
		if err != nil {
			continue
		}
		g, ok := gm.(string)
		if !ok {
			continue
		}
		v, ok := vm.(string)
		if !ok {
			continue
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", c.Class, o, v, g)
		hit++
	}
	fmt.Printf("\nhit/total = %d/%d\n", hit, len(schema.Classes))
}

func aiAdd() {
	if weaviateURL == "" {
		log.Fatalln("you have to specify weaviate url")
	}
	if aiClass == "" {
		log.Fatalln("you have to specify weaviate class name")
	}
	if ollamaURL == "" {
		log.Fatalln("you have to specify ollama url")
	}
	if text2vecModel == "" {
		log.Fatalln("you have to specify text to vector model")
	}
	if generativeModel == "" {
		log.Fatalln("you have to specify generative model")
	}
	client, err := getWeaviateClient()
	if err != nil {
		log.Fatalln(err)
	}
	classObj := &models.Class{
		Class:      aiClass,
		Vectorizer: "text2vec-ollama",
		ModuleConfig: map[string]interface{}{
			"text2vec-ollama": map[string]interface{}{
				"apiEndpoint": ollamaURL,
				"model":       text2vecModel,
			},
			"generative-ollama": map[string]interface{}{
				"apiEndpoint": ollamaURL,
				"model":       generativeModel,
			},
		},
	}
	err = client.Schema().ClassCreator().WithClass(classObj).Do(context.Background())
	if err != nil {
		log.Fatalln(err)
	}
}

func aiDelete() {
	if weaviateURL == "" {
		log.Fatalln("you have to specify weaviate url")
	}
	if aiClass == "" {
		log.Fatalln("you have to specify weaviate class name")
	}
	client, err := getWeaviateClient()
	if err == nil {
		err = client.Schema().ClassDeleter().WithClassName(aiClass).Do(context.Background())
	}
	if err != nil {
		log.Fatalln(err)
	}
}

type aiMsg struct {
	Done  bool
	Lines int
	Hit   int
	Dur   time.Duration
}

type aiAnswer string

func aiTalk() {
	if weaviateURL == "" {
		log.Fatalln("you have to specify weaviate url")
	}
	if aiClass == "" {
		log.Fatalln("you have to specify weaviate class name")
	}
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initAIModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go aiTalkSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		log.Fatalln(err)
	}
	wg.Wait()
}

var aiLogList = []string{}

func aiTalkSub(wg *sync.WaitGroup) {
	defer wg.Done()
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
	i := 0
	db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("logs"))
		c := b.Cursor()
		for k, v := c.Seek([]byte(sk)); k != nil; k, v = c.Next() {
			a := strings.Split(string(k), ":")
			if len(a) < 1 {
				continue
			}
			t, err := strconv.ParseInt(a[0], 16, 64)
			if err == nil && t > eti {
				break
			}
			l := string(v)
			i++
			if matchFilter(&l) {
				hit++
				aiLogList = append(aiLogList, l)
			}
			if i%100 == 0 {
				teaProg.Send(aiMsg{Lines: i, Hit: len(aiLogList), Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	teaProg.Send(aiMsg{Done: true, Lines: i, Hit: len(aiLogList), Dur: time.Since(st)})
}

type aiModel struct {
	spinner       spinner.Model
	table         table.Model
	viewport      viewport.Model
	done          bool
	log           string
	answer        string
	quitting      bool
	msg           aiMsg
	ask           bool
	wait          bool
	teach         bool
	textareaAsk   textarea.Model
	textareaTeach textarea.Model
}

func initAIModel() aiModel {
	columns := []table.Column{
		{Title: "Log"},
	}
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00efff"))
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(7),
	)

	ts := table.DefaultStyles()
	ts.Header = ts.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	ts.Selected = ts.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(ts)
	tad := textarea.New()
	tad.Placeholder = "Descr"
	tad.Focus()
	tad.CharLimit = 4096
	taa := textarea.New()
	taa.Placeholder = "Question"
	taa.Focus()
	taa.CharLimit = 4096
	vp := viewport.New(100, 100)
	return aiModel{spinner: s, table: t, textareaTeach: tad, textareaAsk: taa, viewport: vp}
}

func (m aiModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var lastAICursor = -1

func (m aiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.ask {
		return m.askUpdate(msg)
	}
	if m.teach {
		return m.teachUpdate(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			if m.done {
				return m, tea.Quit
			}
			m.quitting = true
			stopSearch = true
			return m, nil
		case "t":
			if m.done {
				lastAICursor = m.table.Cursor()
				if lastAICursor >= 0 && lastAICursor < len(aiLogList) {
					m.teach = true
				}
			}
			return m, nil
		case "a":
			if m.done {
				lastAICursor = m.table.Cursor()
				if lastAICursor >= 0 && lastAICursor < len(aiLogList) {
					m.ask = true
				}
			}
			return m, nil
		case "enter":
			if m.done {
				if m.log == "" && m.answer == "" {
					w := m.table.Width()
					if sel := m.table.SelectedRow(); sel != nil {
						s := sel[0]
						m.log = wrapString(s, w)
					}
				} else {
					m.log = ""
					m.answer = ""
				}
			}
		default:
			if !m.done {
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.table.SetWidth(msg.Width - 6)
		m.table.SetHeight(msg.Height - 5)
		m.viewport.Height = msg.Height - 5
		m.viewport.Width = msg.Width
		m.textareaAsk.SetWidth(msg.Width - 6)
		m.textareaAsk.SetHeight(msg.Height - 5)
		m.textareaTeach.SetWidth(msg.Width - 6)
		m.textareaTeach.SetHeight(msg.Height - 5)
		columns := []table.Column{
			{Title: "Log", Width: m.table.Width() - 2},
		}
		m.table.SetColumns(columns)
	case aiMsg:
		if msg.Done {
			rows := []table.Row{}
			for _, l := range aiLogList {
				rows = append(rows, table.Row{l})
			}
			m.table.SetRows(rows)
			m.done = true
		}
		m.msg = msg
		return m, nil
	case aiAnswer:
		m.answer = string(msg)
		m.viewport.SetContent(m.answer)
		m.wait = false
	default:
		if !m.done || m.wait {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m aiModel) askUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlS:
			m.wait = true
			go askToAI(m.textareaAsk.Value())
			m.ask = false
			return m, m.spinner.Tick
		case tea.KeyCtrlD:
			m.textareaAsk.SetValue("")
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			m.ask = false
			return m, nil
		}
	}
	m.textareaAsk, cmd = m.textareaAsk.Update(msg)
	return m, cmd
}

func (m aiModel) teachUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlS:
			m.wait = true
			go teachToAI(m.textareaTeach.Value())
			m.teach = false
			return m, m.spinner.Tick
		case tea.KeyCtrlD:
			m.textareaTeach.SetValue("")
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			m.teach = false
			return m, nil
		}
	}
	m.textareaTeach, cmd = m.textareaTeach.Update(msg)
	return m, cmd
}

func (m aiModel) View() string {
	if m.wait {
		return fmt.Sprintf("%s AI is thinking...", m.spinner.View())
	}
	if m.ask {
		return fmt.Sprintf("Enter a question about this log.\n\n%s\n\n(ctl+s: ask / ctl+d: clear / esc: quit)", m.textareaAsk.View())
	}
	if m.teach {
		return fmt.Sprintf("Tell me about this log.\n\n%s\n\n(ctl+s: teach / ctl+d: clear / esc: quit)", m.textareaTeach.View())
	}
	if m.answer != "" {
		return m.viewport.View()
	}
	if m.done {
		if m.log != "" {
			return m.log
		}
		return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.table.View()))
	}
	str := fmt.Sprintf("\nSearch %s line=%s hit=%s time=%v",
		m.spinner.View(),
		humanize.Comma(int64(m.msg.Lines)),
		humanize.Comma(int64(m.msg.Hit)),
		m.msg.Dur,
	)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}

func (m aiModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d s:%s", m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond)))
	help := helpStyle("enter: Show / t: Teach / a: Ask / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func teachToAI(d string) {
	client, err := getWeaviateClient()
	if err != nil {
		teaProg.Send(aiAnswer(err.Error()))
		return
	}
	objects := []*models.Object{
		{
			Class: aiClass,
			Properties: map[string]any{
				"log":   aiLogList[lastAICursor],
				"descr": d,
			},
		},
	}
	batchRes, err := client.Batch().ObjectsBatcher().WithObjects(objects...).Do(context.Background())
	if err != nil {
		teaProg.Send(aiAnswer(err.Error()))
		return
	}
	errs := []string{}
	for _, res := range batchRes {
		if res.Result.Errors != nil {
			errs = append(errs, fmt.Sprintf("%v", res.Result.Errors.Error))
		}
	}
	if len(errs) > 0 {
		teaProg.Send(aiAnswer(strings.Join(errs, "\n")))
	}
	teaProg.Send(aiAnswer(""))
}

func askToAI(p string) {
	client, err := getWeaviateClient()
	if err != nil {
		teaProg.Send(aiAnswer(err.Error()))
		return
	}
	ctx := context.Background()
	gs := graphql.NewGenerativeSearch().GroupedResult(p)
	concepts := []string{aiLogList[lastAICursor]}
	response, err := client.GraphQL().Get().
		WithClassName(aiClass).
		WithFields(
			graphql.Field{Name: "log"},
			graphql.Field{Name: "descr"},
		).
		WithGenerativeSearch(gs).
		WithNearText(client.GraphQL().NearTextArgBuilder().
			WithConcepts(concepts)).
		WithLimit(aiLimit).
		Do(ctx)
	if err != nil {
		teaProg.Send(aiAnswer(err.Error()))
		return
	}
	errs := []string{}
	for _, e := range response.Errors {
		errs = append(errs, fmt.Sprintf("%v", e))
	}
	if len(errs) > 0 {
		teaProg.Send(aiAnswer(strings.Join(errs, "\n")))
		return
	}
	r, err := jsonpath.Get("$..groupedResult", response.Data["Get"])
	if err != nil {
		teaProg.Send(aiAnswer(err.Error()))
		return
	}
	teaProg.Send(aiAnswer(fmt.Sprintf("%v", r)))
}

func getWeaviateClient() (*weaviate.Client, error) {
	u, err := url.Parse(weaviateURL)
	if err != nil {
		return nil, err
	}
	cfg := weaviate.Config{
		Host:   u.Host,
		Scheme: u.Scheme,
	}
	return weaviate.NewClient(cfg)
}
