# twsla 組み込み Sigma / Wazuh ルールパック詳細ガイド

`twsla` には、セキュリティログ分析・脅威ハンティングにおいて即効性が高く誤検知の少ない **11種類の推奨ルールパック（合計75ルール）** が標準で組み込まれています。
外部からルールファイルをダウンロード・配置することなく、`twsla sigma` を実行するだけですぐに高精度な脅威検知を開始できます。

---

## 目次

1. [ルールパックの概要と使い方](#1-ルールパックの概要と使い方)
2. [ルールパック一覧](#2-ルールパック一覧)
3. [各パックの詳細と収録ルール](#3-各パックの詳細と収録ルール)
   - [windows-essential (Windows標準重要イベント)](#windows-essential)
   - [windows-ad (Active Directory / ドメイン侵害)](#windows-ad)
   - [windows-client (Windows端末・エンドポイント脅威)](#windows-client)
   - [linux-auth (Linux認証・特権昇格)](#linux-auth)
   - [linux-system (Linux永続化・システム改ざん)](#linux-system)
   - [network-threats (ネットワーク機器・FW・VPN)](#network-threats)
   - [web-attacks (Webサーバー・プロキシ攻撃)](#web-attacks)
   - [wazuh-compliance (コンプライアンス監査・統制基準)](#wazuh-compliance)
   - [wazuh-linux (Wazuh変換 Linuxルール)](#wazuh-linux)
   - [wazuh-network (Wazuh変換 ネットワークルール)](#wazuh-network)
   - [wazuh-web (Wazuh変換 Webルール)](#wazuh-web)
4. [カスタムルールとの併用と優先度オーバーライド](#4-カスタムルールとの併用と優先度オーバーライド)
5. [Wazuh ルール・デコーダーの変換コマンド](#5-wazuh-ルールデコーダーの変換コマンド)
6. [ログのタイムスタンプに基づく相関検知](#6-ログのタイムスタンプに基づく相関検知)
7. [TUI でのタグ別集計とグラフ表示](#7-tui-でのタグ別集計とグラフ表示)

---

## 1. ルールパックの概要と使い方

### 基本実行（全パック自動有効）
オプションなしで `twsla sigma` を実行すると、組み込みの全ルールパック（75ルール）が自動的に適用されます：

```bash
twsla sigma
```

### 特定パックに絞り込む指定
`--sigmaPacks`（`-p`）オプションでカンマ区切りでパック名を指定します：

```bash
twsla sigma -p windows-essential,linux-auth
```

### CLI による確認とテスト

```bash
# 利用可能なパックの一覧・ルール数・説明を表示
twsla sigma packs

# 有効化されているルールをパック別・出所別に一覧表示
twsla sigma list -p linux-auth

# ルール総数、ログソース数、フィールド数、Top Tags（MITRE ATT&CK / 規格別）の統計を表示
twsla sigma stat

# ルールのマッチングテスト
twsla sigma test '{"Event":{"System":{"Channel":"Security","Computer":"WIN-DC","EventID":4625,"Level":0},"EventData":{"TargetUserName":"admin","WorkstationName":"DESKTOP-1"}}}'
```

---

## 2. ルールパック一覧

| パック名 | ルール数 | 対象ログソース | 主な検知対象・目的 |
| :--- | :---: | :--- | :--- |
| **`windows-essential`** | 12 | Windows Event (Security, System, Defender) | ログオン失敗、ログ消去、新規サービス登録、Defender無効化、PowerShell難読化等 |
| **`windows-ad`** | 8 | Windows Event (Security / AD DC) | Kerberoasting、AS-REP Roasting、DCSync、ドメイン信頼関係改変、GPO変更等 |
| **`windows-client`** | 8 | Windows Event (Security, TerminalServices) | 不審なRDP接続、UACバイパス、USBストレージ接続、LSASSメモリダンプ兆候等 |
| **`linux-auth`** | 6 | Linux Syslog (sshd, sudo, useradd) | SSHブルートフォース、不正ユーザーSSH、root直接ログイン、sudo認証失敗等 |
| **`linux-system`** | 7 | Linux Syslog (cron, systemd, shadow, ufw) | cronジョブ改変、systemd追加、passwd/shadow改ざん、ファイアウォール停止等 |
| **`network-threats`** | 8 | Syslog (Fortinet, Cisco, Yamaha, Palo Alto) | VPN認証失敗、管理WebUI/SSHログイン失敗、ポートスキャン、コンフィグ変更等 |
| **`web-attacks`** | 8 | Web/Proxy アクセスログ (Syslog / OTel) | Log4Shell、パストラバーサル、SQLi、WebShell、脆弱性スキャナーUA、XSS等 |
| **`wazuh-compliance`** | 8 | Syslog / Linux Audit / PAM | PCI-DSS, NIST SP 800-53, GDPR, CIS Controls 準拠の重要セキュリティ監査項目 |
| **`wazuh-linux`** | 6 | Linux Syslog (sshd, sudo, pam) | Wazuh ルールセットから変換された Linux 認証・権限昇格検知（相関検知対応） |
| **`wazuh-network`** | 2 | Syslog (Cisco, Fortinet) | Wazuh ルールセットから変換された Cisco / Fortinet VPN ブルートフォース |
| **`wazuh-web`** | 2 | Web アクセスログ (Apache, Nginx) | Wazuh ルールセットから変換された脆弱性スキャン・機密ファイル探索 |

---

## 3. 各パックの詳細と収録ルール

### `windows-essential`
Windows サーバーやクライアントの**標準イベントログ（Sysmon 等の追加エージェント不要）**で検知できる最重要セキュリティイベントを集約したパックです。

| ルールタイトル | 対象イベント / 条件 | レベル | 目的とセキュリティ効果 | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Windows Failed Logon Attempt** | EventID 4625 (Security) | `low` | パスワード総当たり（ブルートフォース）やパスワードスプレー攻撃を検知。 | T1110 (Credential Access) |
| **User Account Created** | EventID 4720 (Security) | `medium` | 不正なローカルアカウント作成による永続化（バックドア）を検知。 | T1136.001 (Persistence) |
| **Member Added to Security Group** | EventID 4728, 4732, 4756 | `high` | Administrators などの特権グループへの不正なメンバー追加を検知。 | T1098 (Privilege Escalation) |
| **Special Privileges Assigned to New Logon** | EventID 4672 (Security) | `low` | 管理者権限（SeDebugPrivilege 等）の行使を監視・監査。 | T1078 (Privilege Escalation) |
| **Security Event Log Cleared** | EventID 1102 (Security) | `high` | 攻撃者による侵入痕跡消去（監査ログクリア）を検知。 | T1070.001 (Defense Evasion) |
| **System Event Log Cleared** | EventID 104 (System) | `high` | システムイベントログのクリアによる痕跡消去を検知。 | T1070.001 (Defense Evasion) |
| **Windows Defender Real-time Protection Disabled** | EventID 5001 (WinDefend) | `high` | マルウェア実行前にアンチウイルス保護が無効化された異常を検知。 | T1562.001 (Defense Evasion) |
| **New Windows Service Installed** | EventID 7045 (System) | `medium` | PsExec や悪意あるバックドアサービスの新規インストールを検知。 | T1543.003 (Persistence) |
| **Scheduled Task Created** | EventID 4698 (Security) | `medium` | スケジュールタスクの登録による自動起動・永続化を検知。 | T1053.005 (Persistence) |
| **Suspicious PowerShell Execution** | EventID 4688 (`-enc`, `DownloadString`, `iex`) | `high` | 難読化 PowerShell や外部からのインメモリコード実行を検知。 | T1059.001 (Execution) |
| **Certutil Remote File Download** | EventID 4688 (`certutil` + `urlcache`) | `high` | LOLBAS（標準コマンドの悪用）による外部からのマルウェアダウンロードを検知。 | T1105 (Command and Control) |
| **Volume Shadow Copies Deletion Via Vssadmin** | EventID 4688 (`vssadmin` + `delete shadows`) | `critical` | ランサムウェアによるバックアップ（シャドウコピー）削除活動を検知。 | T1490 (Impact) |

---

### `windows-ad`
Active Directory ドメイン環境における**ドメインコントローラ特有の脅威や横展開（Lateral Movement）**を検知するパックです。

| ルールタイトル | 対象イベント / 条件 | レベル | 目的とセキュリティ効果 | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Potential Kerberoasting Attack** | EventID 4769 (TicketEncryptionType: 0x17) | `high` | SPN アカウントに対する弱い RC4 チケット要求（オフラインパスワード解析狙い）を検知。 | T1558.003 (Credential Access) |
| **Potential AS-REP Roasting Attack** | EventID 4768 (PreAuthType: 0) | `medium` | 事前認証不要アカウントに対するチケット要求（オフライン解析狙い）を検知。 | T1558.004 (Credential Access) |
| **Potential DCSync Active Directory Replication** | EventID 4662 (AccessMask: 0x100) | `high` | Mimikatz 等によるドメイン管理者パスワードハッシュ一括抽出（DCSync）を検知。 | T1003.006 (Credential Access) |
| **Domain Trust Relationship Modified** | EventID 4706, 4716 | `high` | ドメイン信頼関係の不正な作成・変更（悪意ある外部ドメインとの結合）を検知。 | T1484 (Persistence) |
| **User Password Reset Attempt** | EventID 4724 | `low` | 管理者によるパスワード強制リセットの集中発生を監視。 | T1098 (Persistence) |
| **User Account Unlocked** | EventID 4767 | `low` | ブルートフォース等でロックされたアカウントの解除操作を監査。 | T1098 (Persistence) |
| **Computer Account Created in Domain** | EventID 4741 | `low` | ドメインへの新規コンピュータ追加（偽装マシンや踏み台）を検知。 | T1136.002 (Persistence) |
| **Group Policy Object Modified** | EventID 5136, 5137 | `medium` | ドメイン全体に悪意あるスクリプトや設定を配布する GPO 改ざんを検知。 | T1484.001 (Persistence) |

---

### `windows-client`
クライアント PC 端末やリモートワーク端末における**不審な操作・侵入拡大活動**を検知するパックです。

| ルールタイトル | 対象イベント / 条件 | レベル | 目的とセキュリティ効果 | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Remote Desktop (RDP) Logon Detected** | EventID 4624 (LogonType 10) | `low` | リモートデスクトップ経由での対話型ログオンを監視・可視化。 | T1021.001 (Lateral Movement) |
| **Terminal Services RDP Connection Established** | EventID 1149 (TerminalServices) | `low` | RDP ネットワーク接続の確立を追跡。 | T1021.001 (Lateral Movement) |
| **UAC Bypass Via Fodhelper** | EventID 4688 (`fodhelper`) | `high` | ユーザーへの確認ダイアログを出さずに管理者権限へ昇格する手口を検知。 | T1548.002 (Privilege Escalation) |
| **UAC Bypass Via Event Viewer** | EventID 4688 (`eventvwr.exe`) | `low` | レジストリハイジャックを用いた UAC バイパスの試行を検知。 | T1548.002 (Privilege Escalation) |
| **USB Storage Device Plugged In** | EventID 20001, 20003 | `low` | 私物 USB や未許可ストレージの接続による情報漏洩・マルウェア侵入を監視。 | T1052.001 (Initial Access / Exfiltration) |
| **LSASS Memory Dump Attempt** | EventID 4688 (`lsass` + `dump`) | `critical` | procdump 等で LSASS プロセスメモリをダンプし平文パスワードを盗む手口を検知。 | T1003.001 (Credential Access) |
| **BITSAdmin File Transfer Execution** | EventID 4688 (`bitsadmin` + `/transfer`) | `medium` | バックグラウンドインテリジェント転送サービスを悪用した不正ファイル取得を検知。 | T1197 (Defense Evasion) |
| **Network Share Object Added** | EventID 5142 | `low` | マルウェア配布や横展開目的の不審な共有フォルダ作成を検知。 | T1021.002 (Lateral Movement) |

---

### `linux-auth`
Linux サーバーに対する**SSH ログインや sudo などの認証・特権昇格イベント**を検知するパックです。

| ルールタイトル | 対象キーワード / 条件 | レベル | 目的とセキュリティ効果 | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **SSH Failed Password Authentication** | `Failed password for` | `low` | SSH パスワード認証失敗（ブルートフォース攻撃）を検知。 | T1110 (Credential Access) |
| **SSH Login Attempt With Invalid User** | `Failed password for invalid user` | `medium` | 存在しないユーザー名でのログイン試行（アカウント探索攻撃）を検知。 | T1110 (Credential Access) |
| **Direct SSH Root Login Accepted** | `Accepted password for root` / `Accepted publickey for root` | `medium` | 本来禁止されるべき root アカウントへの直接 SSH ログイン成功を監視。 | T1078.003 (Initial Access) |
| **Sudo Authentication Failure** | `authentication failure`, `incorrect password attempt` | `medium` | 一般ユーザーからの不正な sudo 実行や内部不正を検知。 | T1548.003 (Privilege Escalation) |
| **Sudoers Configuration File Modified** | `etc/sudoers`, `visudo` | `high` | 特権昇格ルールの改ざん（NOPASSWD 付与など）を検知。 | T1548.003 (Privilege Escalation) |
| **New Linux User Account Created** | `new user: name=`, `useradd` | `medium` | 不正なバックドアアカウントの追加を検知。 | T1136.001 (Persistence) |

---

### `linux-system`
Linux サーバーへの侵入後に行われる**「永続化」「設定改ざん」「セキュリティ停止」**を検知するパックです。

| ルールタイトル | 対象キーワード / 条件 | レベル | 目的とセキュリティ効果 | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Cron Job Created or Modified** | `REPLACE (`, `crontab[`, `/etc/cron` | `low` | cron による定期実行タスクの登録（バックドアの再接続など）を検知。 | T1053.003 (Persistence) |
| **Systemd Service Installed or Started** | `Created symlink /etc/systemd/system/`, `systemd: Started` | `low` | systemd サービスファイルの配置による自動起動登録を検知。 | T1543.002 (Persistence) |
| **Password File Modified or Password Changed** | `password changed for`, `/etc/shadow` | `low` | パスワード変更やシャドウパスワードファイルの改変を監査。 | T1098 (Persistence) |
| **Linux Host Firewall Stopped or Flushed** | `ufw stop`, `firewalld: stopped`, `iptables -F` | `medium` | 防御壁（ホストファイアウォール）の無効化を検知。 | T1562.004 (Defense Evasion) |
| **Linux User Account Deleted** | `delete user`, `userdel` | `low` | 痕跡消去や業務妨害目的のアカウント削除を検知。 | T1531 (Impact) |
| **New Linux Group Created** | `new group: name=`, `groupadd` | `low` | 不審な新規グループ作成を検知。 | T1136.001 (Persistence) |
| **Unsigned or Out-of-Tree Kernel Module Loaded** | `loading out-of-tree module`, `module verification failed` | `medium` | ルートキット等の未検証カーネルモジュールロードを検知。 | T1547.006 (Persistence) |

---

### `network-threats`
Fortinet (FortiGate)、Cisco (IOS / ASA)、Yamaha ルーター、Palo Alto 等の**ネットワーク機器・UTM・ファイアウォールログ**を監視するパックです。

| ルールタイトル | 対象キーワード / 条件 | レベル | 目的とセキュリティ効果 | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **VPN Authentication Failed** | `VPN authentication failed`, `IPsec authentication failed` | `medium` | リモートアクセス VPN への認証総当たりを検知。 | T1110 (Initial Access) |
| **FortiGate SSL VPN Authentication Failure** | `action=ssl-login-fail`, `SSL VPN login fail` | `medium` | FortiGate 機器への SSL-VPN ログイン失敗を検知。 | T1110 (Initial Access) |
| **Cisco Device Authentication Failure** | `%SEC_LOGIN-4-LOGIN_FAILED`, `%AAA-3-BADAUTH` | `medium` | Cisco ルーター/スイッチ/ASA への不正ログインを検知。 | T1110 (Credential Access) |
| **Yamaha Router Authentication Failure** | `Login failed`, `rejected TELNET`, `rejected SSH` | `medium` | Yamaha ルーター（RTX/NVRシリーズ）への不正アクセスを検知。 | T1110 (Credential Access) |
| **Firewall Admin Console Login Failure** | `admin login failed`, `WebUI login failed` | `medium` | ファイアウォール管理画面（GUI/SSH）への侵入試行を検知。 | T1110 (Credential Access) |
| **Network Scan or Deny Flood** | `port scan`, `SYN flood`, `IP spoofing`, `LAND attack` | `high` | 外部からの偵察スキャン活動や DoS 攻撃の兆候を検知。 | T1046 (Discovery) |
| **Palo Alto Networks Threat Detected** | `,THREAT,`, `,vulnerability,`, `,spyware,` | `high` | Palo Alto NGFW が検知した既知のマルウェアや脅威ログを通知。 | T1190 (Initial Access) |
| **Network Device Configuration Changed** | `configuration changed`, `config commit`, `save config` | `low` | ルーター・FW の設定変更ログを自動監査。 | T1565 (Defense Evasion) |

---

### `web-attacks`
Apache、Nginx、リバースプロキシ等の**Web アクセスログに現れる典型的な攻撃パターン**を検知するパックです。

| ルールタイトル | 対象キーワード / 条件 | レベル | 目的とセキュリティ効果 | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Log4j JNDI Exploit Attempt (Log4Shell)** | `${jndi:ldap:`, `${jndi:rmi:`, `${jndi:dns:` | `critical` | 深刻な脆弱性 Log4Shell (CVE-2021-44228) の悪用試行を確実に検知。 | T1190 (Initial Access) |
| **Path Traversal Attempt** | `/../`, `/etc/passwd`, `win.ini`, `%2e%2e%2f` | `high` | サーバー内の機密ファイル閲覧を狙うディレクトリスキームを検知。 | T1190 (Initial Access) |
| **SQL Injection Attempt** | `UNION SELECT`, `' OR 1=1`, `WAITFOR DELAY` | `high` | データベース奪取を狙う SQL インジェクション攻撃を検知。 | T1190 (Initial Access) |
| **WebShell Access or Execution** | `eval(base64_decode`, `c99.php`, `r57.php`, `wso.php` | `critical` | 設置されたバックドア（WebShell）へのアクセスや遠隔操作を検知。 | T1505.003 (Persistence) |
| **Web Vulnerability Scanner User-Agent** | `Nikto`, `sqlmap`, `gobuster`, `dirbuster`, `Acunetix` | `medium` | 自動攻撃ツールによる事前調査・脆弱性探索アクティビティを検知。 | T1595.002 (Discovery) |
| **Cross Site Scripting (XSS) Pattern** | `<script>`, `javascript:alert(`, `<svg/onload=` | `medium` | クライアント攻撃を狙う XSS ペイロードの送信を検知。 | T1190 (Initial Access) |
| **Spring4Shell Remote Code Execution Attempt** | `class.module.classLoader` | `critical` | Spring Framework の RCE 脆弱性 (CVE-2022-22965) 悪用を検知。 | T1190 (Initial Access) |
| **PHP Information Disclosure Access (phpinfo)** | `phpinfo.php`, `info.php`, `?phpinfo=` | `low` | 攻撃者によるサーバー環境情報の偵察活動を検知。 | T1592.002 (Discovery) |

---

### `wazuh-compliance`
PCI-DSS, NIST SP 800-53, GDPR, CIS Controls などの**主要セキュリティ・コンプライアンス監査基準**に対応したパックです。

| ルールタイトル | 対象イベント / 条件 | レベル | 該当規格項目 |
| :--- | :--- | :---: | :--- |
| **Audit Log Daemon (auditd) Stopped** | `auditd: stopped`, `auditd.*termination` | `high` | PCI-DSS 10.2.6, NIST AU-12, GDPR 35.7.d, CIS 8.5 |
| **System Logging Daemon Stopped** | `syslogd: stopped`, `rsyslogd: stopped` | `high` | PCI-DSS 10.2.6, NIST AU-12, GDPR 35.7.d, CIS 8.5 |
| **Superuser (root) Elevation Failure** | `BAD SU`, `su:.*FAILED` | `medium` | PCI-DSS 10.2.4, NIST AC-6, CIS 5.4 |
| **Sudoers File Unauthorized Modification** | `sudoers modified`, `/etc/sudoers` | `high` | PCI-DSS 10.2.2, NIST CM-5, CIS 5.2 |
| **User Account Locked Out** | `account locked`, `pam_tally` | `low` | PCI-DSS 8.1.6, NIST AC-7, CIS 5.3 |
| **User Added to Administrative Group** | `add .* to group (wheel\|sudo\|admin)` | `medium` | PCI-DSS 10.2.5, NIST AC-2, CIS 5.1 |
| **System Clock Synchronization Skew** | `time reset`, `ntpdate.*step time` | `medium` | PCI-DSS 10.4, NIST AU-8, CIS 6.1 |
| **Database Access Authentication Denied** | `Access denied for user`, `pg_hba.conf rejects` | `medium` | PCI-DSS 10.2.4, NIST AC-6 |

---

### `wazuh-linux`, `wazuh-network`, `wazuh-web`
Wazuh ルールセットから変換されたルールパックです。

- **`wazuh-linux`** (6件): SSHD 不正ユーザー認証、ブルートフォース攻撃（相関検知: 120秒以内に同一IPから6回）、root ログイン検知、sudo 実行、sudoers 未登録実行、PAM 認証失敗。
- **`wazuh-network`** (2件): Cisco 管理画面ログイン失敗、FortiGate SSL-VPN 複数回連続認証失敗（相関検知対応）。
- **`wazuh-web`** (2件): 脆弱性スキャナー探索、機密ファイル (`.git`, `.env`, `.htpasswd`) 探索。

---

## 4. カスタムルールとの併用と優先度オーバーライド

独自の Sigma ルールファイル（`--rules`）を組み込みパックと併用できます。

### 優先度ルール
同じ `id`（UUID）を持つルールが重複した場合、**外部ファイル側のルールが組み込みパックを自動的に上書き（オーバーライド）**します：

1. **`file:`**（高優先度: `--rules` で指定した外部カスタムルール）
2. **`pack:`**（標準優先度: 組み込みルールパック）

### 活用例（ルールのチューニング）
組み込みパックのルール（例: `018d9f10-ff31-419b-a36c-941cbfa0451a`）を自社要件に合わせて「社内 IP を除外したい」「重要度レベルを `critical` に引き上げたい」場合、同じ ID を持つ YAML ファイルを作成して `--rules` に指定するだけで、組み込みパックのルールがカスタムルールへ安全に置き換わります。

---

## 5. Wazuh ルール・デコーダーの変換コマンド

### Wazuh ルール XML から Sigma YAML への変換 (`convert-wazuh`)
手元の Wazuh ルール XML ファイルやリポジトリのルールセットを、Sigma YAML ルールへ変換できます。`<if_sid>` などの親子依存関係は自動で AND 展開され、`<frequency>` / `<timeframe>` による相関条件も保持されます。

```bash
# 単一の Wazuh ルール XML を Sigma YAML に変換
twsla sigma convert-wazuh -o ./converted-rules ./0095-sshd_rules.xml

# ディレクトリ内の全 XML を一括変換
twsla sigma convert-wazuh -o ./converted-rules /path/to/wazuh/rules/

# 標準出力へ YAML を出力（パイプ処理用）
twsla sigma convert-wazuh --stdout ./0095-sshd_rules.xml
```

### Wazuh デコーダー XML から正規表現への変換 (`convert-wazuh-decoder`)
Wazuh デコーダー定義（`<regex>`, `<order>`）から、`--namedCaptures` で使用できる Go の名前付きキャプチャグループ正規表現を自動生成します。

```bash
# デコーダー XML を変換してファイルに出力
twsla sigma convert-wazuh-decoder -o ./captures ./0310-ssh_decoders.xml
```

---

## 6. ログのタイムスタンプに基づく相関検知

Sigma ルールに含まれる `correlation:` メタデータ（`frequency`, `timeframe`, `group_by`）を `twsla` の検知エンジンが解釈します。

- **タイムスタンプ基準の計算**:
  リアルタイム受信だけでなく、**過去にインポートした蓄積ログに対しても、ログ記録時のタイムスタンプ（`entry.Time`）に沿ってスライディング時間窓を正確に再現・判定**します。
- **同一キーのグルーピング**:
  `group_by: ["client"]` 等により、同一送信元 IP ごとに発生回数を集計し、指定時間枠内に指定回数以上イベントが発生した時点で即座に検知されます。

---

## 7. TUI でのタグ別集計とグラフ表示

`twsla sigma` の TUI 画面では、キーボード操作で素早く脅威の傾向を把握できます。

- **モード切り替え (`c` キー)**:
  `c` キーを押すことで、**「イベント一覧」 ⇄ 「ルール別集計」 ⇄ 「タグ別集計（MITRE ATT&CK / 規格別）」** が順次切り替わります。
- **タグ別グラフ表示 (`h` / `g` キー)**:
  タグ別集計画面を表示中に `h`（ブラウザで ECharts）または `g`（Sixel / PNG 端末表示）を押すことで、**最も多く検知された攻撃手法やコンプライアンス違反項目の棒グラフ**を表示・保存できます。
- **ファイル保存 (`s` キー)**:
  表示中のモード（イベント / ルール / タグ）に合わせた TSV 形式ファイルやグラフ画像を出力できます。
