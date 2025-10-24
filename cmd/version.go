/*
Copyright © 2024 Masayuki Yamai <twsnmp@gmail.com>

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
	"fmt"
	"strings"

	"github.com/common-nighthawk/go-figure"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var Version string
var Commit string
var Date string
var versionColor string

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show twsla version",
	Long:  `Show twsla version`,
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().StringVar(&versionColor, "color", "", "Version color")
}

func printVersion() {
	f := figure.NewFigure("TWSLA", "roman", true)
	fs := f.String()
	logoColor := color.New(color.Bold)
	catColor := color.New((color.BgBlack)).Add(color.Bold)
	switch versionColor {
	case "blue":
		logoColor.Add(color.FgBlue)
		catColor.Add((color.FgBlue))
	case "yellow":
		logoColor.Add(color.FgYellow)
		catColor.Add((color.FgYellow))
	case "magenta":
		logoColor.Add(color.FgMagenta)
		catColor.Add((color.FgMagenta))
	case "cyan":
		logoColor.Add(color.FgCyan)
		catColor.Add((color.FgCyan))
	case "green":
		logoColor.Add(color.FgGreen)
		catColor.Add((color.FgGreen))
	case "red":
		logoColor.Add(color.FgRed)
		catColor.Add((color.FgRed))
	default:
		logoColor.Add(color.FgHiWhite)
		catColor.Add((color.FgHiWhite))
	}
	logoColor.Println(strings.TrimSpace(fs))
	catColor.Println(cat + fmt.Sprintf("twsla v%s(%s) %s\n", Version, Commit, Date))
}

var cat = `@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@@@@@@@@@@@*++=##%%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@@@@@@@@@@@@=   .......+%%@@@@@@@@@@@@@@@@@@@@@@@@
@@@@@@@@@@@@@.  .... .**+*@@@@@@@@@@@@@@@@@@@@@@@@
@@@@@@@@@@@@:.. :-  ...:++.@@@@@@@@@@@@@@@@@@@@@@@
@@@@@@@@@@@@:..-@#=:.... :*%@@@@@@@@@@@@@@@@@@@@@@
@@@@@@@@@@@@+:*%%**=...:*####-..::+#%@@@@@@@@@@@@@
@@@@@@@@@@@@@@@%##%%%#%%%#####=.  ....-@@@@@@@@@@@
@@@@@@@@@@@@@@#*##%##%%#####%#*.   .... +@@@@@@@@@
@@@@@@@@@@@@@@@@@%%%%%%%%%%###*.    ...  =@@@@@@@@
@@@@@@@@@@@@@@@@@@@%%%%%%%%###-.   .:-:   +@@@@@@@
@@@@@@@@@@@@@@@@@@@@%%%%%####+.  .:=*-..  .@@@@@@@
@@@@@@@@@@@@@@@@@@%%%%%%##%###-:-*##=..    @@@@@@@
@@@@@@@@@@@@@@@@%%%%%%%%%%##%%%%%%*=..     @@@@@@@
@@@@@@@@@@@@@@@@%%%%%%%%%##%%%%%%*-..     -@@@@@@@
@@@@@@@@@@@@@@@@@%%%%%%%#%%%%%%#-.        *@@@@@@@
@@@@@@@@@@@@@@@@@@%%%%%%%%%%%+:.         .@@@@@@@@
@@@@@@@@@@@@@@@@@@@%%%%%%%#*+=:.......   %@@@@@@@@
@@@@@@@@@@@@@@@@@@@@@%%@@%%%%%##=.....  -@@@@@@@@@
@@@@@@@@@@@@@@@@@@@@@%@@@%%%%%%%####+. .#@@@@@@@@@
@@@@@@@@@@@@@@@@@@@@%%@@@%%%%%%%%%%###*#@@@@@@@@@@
@@@@@@@@@@@@@@@@@%%@%%%@@@%%%%%%######*@@@@@@@@@@@
@@@@@@@@@@@@@@@@%@@%%#%@%%%%###%%%##*%#%@@@@@@@@@@
@@@@@@@@@@@@@@@@@@%%%@@%%%#**######**@@@@@@@@@@@@@
`
