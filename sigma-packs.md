# twsla Embedded Sigma and Wazuh Rule Packs Guide

`twsla` provides **11 recommended embedded Sigma rule packs (75 rules in total)** designed for high-fidelity detection with minimal false positives.
Without needing to download or configure external rule repositories, simply running `twsla sigma` automatically loads all embedded packs to immediately start threat detection.

---

## Table of Contents

1. [Overview and Usage](#1-overview-and-usage)
2. [Rule Packs Summary](#2-rule-packs-summary)
3. [Detailed Pack and Rule Reference](#3-detailed-pack-and-rule-reference)
   - [windows-essential (Windows Essential Security Events)](#windows-essential)
   - [windows-ad (Active Directory / Domain Controller Threats)](#windows-ad)
   - [windows-client (Windows Endpoint & Client Threats)](#windows-client)
   - [linux-auth (Linux Authentication & Privilege Escalation)](#linux-auth)
   - [linux-system (Linux Persistence & System Tampering)](#linux-system)
   - [network-threats (Network Devices, Firewalls & VPNs)](#network-threats)
   - [web-attacks (Web Server & Proxy Exploits)](#web-attacks)
   - [wazuh-compliance (Compliance & Security Hardening Audit)](#wazuh-compliance)
   - [wazuh-linux (Wazuh Converted Linux Rules)](#wazuh-linux)
   - [wazuh-network (Wazuh Converted Network Rules)](#wazuh-network)
   - [wazuh-web (Wazuh Converted Web Rules)](#wazuh-web)
4. [Custom Rules and Priority Override](#4-custom-rules-and-priority-override)
5. [Wazuh Conversion Commands](#5-wazuh-conversion-commands)
6. [Timestamp-based Correlation Detection](#6-timestamp-based-correlation-detection)
7. [Tag-based Aggregation and Charting in TUI](#7-tag-based-aggregation-and-charting-in-tui)

---

## 1. Overview and Usage

### Default Execution (All Packs Enabled)
Running `twsla sigma` without arguments automatically activates all 11 embedded rule packs (75 rules):

```bash
twsla sigma
```

### Filtering Specific Packs
Specify desired packs with the `--sigmaPacks` (`-p`) flag:

```bash
twsla sigma -p windows-essential,linux-auth
```

### CLI Inspection and Testing

```bash
# List all available embedded packs and rule counts
twsla sigma packs

# List loaded rules with source and tags
twsla sigma list -p linux-auth

# Show statistics across rules, log sources, fields, and top MITRE / compliance tags
twsla sigma stat

# Test matching against a sample log entry
twsla sigma test '{"Event":{"System":{"Channel":"Security","Computer":"WIN-DC","EventID":4625,"Level":0},"EventData":{"TargetUserName":"admin","WorkstationName":"DESKTOP-1"}}}'
```

---

## 2. Rule Packs Summary

| Pack Name | Rules | Target Log Source | Primary Detection Purpose |
| :--- | :---: | :--- | :--- |
| **`windows-essential`** | 12 | Windows Event (Security, System, Defender) | Logon failure, log cleared, service installed, Defender disabled, PowerShell obfuscation |
| **`windows-ad`** | 8 | Windows Event (Security / AD DC) | Kerberoasting, AS-REP Roasting, DCSync, domain trust modification, GPO tampering |
| **`windows-client`** | 8 | Windows Event (Security, TerminalServices) | Suspicious RDP, UAC bypass, USB media plugged, LSASS memory dump signs |
| **`linux-auth`** | 6 | Linux Syslog (sshd, sudo, useradd) | SSH brute-force, invalid user login, direct root SSH, sudo authentication failure |
| **`linux-system`** | 7 | Linux Syslog (cron, systemd, shadow, ufw) | Cron modification, systemd unit added, shadow tampering, firewall stopped |
| **`network-threats`** | 8 | Syslog (Fortinet, Cisco, Yamaha, Palo Alto) | VPN login failure, admin console attack, port scan / flood, config modification |
| **`web-attacks`** | 8 | Web / Proxy logs (Syslog / OTel) | Log4Shell, directory traversal, SQL injection, web shells, scanner UA, XSS |
| **`wazuh-compliance`** | 8 | Syslog / Linux Audit / PAM | PCI-DSS, NIST SP 800-53, GDPR, and CIS Controls compliance audit rules |
| **`wazuh-linux`** | 6 | Linux Syslog (sshd, sudo, pam) | Converted Wazuh Linux authentication & privilege rules with sliding-window correlation |
| **`wazuh-network`** | 2 | Syslog (Cisco, Fortinet) | Converted Wazuh Cisco / Fortinet authentication failure & brute force |
| **`wazuh-web`** | 2 | Web Access Log (Apache, Nginx) | Converted Wazuh vulnerability scanner and hidden file probing |

---

## 3. Detailed Pack and Rule Reference

### `windows-essential`
Essential security monitoring for Windows environments using standard event logs (no Sysmon required).

| Rule Title | Event / Condition | Level | Security Purpose | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Windows Failed Logon Attempt** | EventID 4625 (Security) | `low` | Detects brute-force and password spraying attacks. | T1110 (Credential Access) |
| **User Account Created** | EventID 4720 (Security) | `medium` | Detects local backdoor user account creation. | T1136.001 (Persistence) |
| **Member Added to Security Group** | EventID 4728, 4732, 4756 | `high` | Detects privilege escalation into Administrators. | T1098 (Privilege Escalation) |
| **Special Privileges Assigned to New Logon** | EventID 4672 (Security) | `low` | Audits administrative logon privileges (e.g. SeDebugPrivilege). | T1078 (Privilege Escalation) |
| **Security Event Log Cleared** | EventID 1102 (Security) | `high` | Detects adversary wiping audit trail logs. | T1070.001 (Defense Evasion) |
| **System Event Log Cleared** | EventID 104 (System) | `high` | Detects system event log clearing. | T1070.001 (Defense Evasion) |
| **Windows Defender Real-time Protection Disabled** | EventID 5001 (WinDefend) | `high` | Detects antivirus protection being disabled. | T1562.001 (Defense Evasion) |
| **New Windows Service Installed** | EventID 7045 (System) | `medium` | Detects PsExec or persistent service installation. | T1543.003 (Persistence) |
| **Scheduled Task Created** | EventID 4698 (Security) | `medium` | Detects persistence via scheduled task creation. | T1053.005 (Persistence) |
| **Suspicious PowerShell Execution** | EventID 4688 (`-enc`, `DownloadString`, `iex`) | `high` | Detects obfuscated or in-memory PowerShell execution. | T1059.001 (Execution) |
| **Certutil Remote File Download** | EventID 4688 (`certutil` + `urlcache`) | `high` | Detects LOLBAS remote payload download. | T1105 (Command and Control) |
| **Volume Shadow Copies Deletion Via Vssadmin** | EventID 4688 (`vssadmin` + `delete shadows`) | `critical` | Detects ransomware backup deletion activity. | T1490 (Impact) |

---

### `windows-ad`
Active Directory and Domain Controller threat detection.

| Rule Title | Event / Condition | Level | Security Purpose | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Potential Kerberoasting Attack** | EventID 4769 (TicketEncryptionType: 0x17) | `high` | Detects RC4 ticket requests for offline cracking. | T1558.003 (Credential Access) |
| **Potential AS-REP Roasting Attack** | EventID 4768 (PreAuthType: 0) | `medium` | Detects roasting on accounts with pre-auth disabled. | T1558.004 (Credential Access) |
| **Potential DCSync Active Directory Replication** | EventID 4662 (AccessMask: 0x100) | `high` | Detects credential dumping via directory replication. | T1003.006 (Credential Access) |
| **Domain Trust Relationship Modified** | EventID 4706, 4716 | `high` | Detects rogue domain trust modifications. | T1484 (Persistence) |
| **User Password Reset Attempt** | EventID 4724 | `low` | Audits administrative password reset activity. | T1098 (Persistence) |
| **User Account Unlocked** | EventID 4767 | `low` | Audits user account unlock operations. | T1098 (Persistence) |
| **Computer Account Created in Domain** | EventID 4741 | `low` | Detects unauthorized machine account creation. | T1136.002 (Persistence) |
| **Group Policy Object Modified** | EventID 5136, 5137 | `medium` | Detects GPO tampering for malicious script deployment. | T1484.001 (Persistence) |

---

### `windows-client`
Endpoint, workstation, and client threat detection.

| Rule Title | Event / Condition | Level | Security Purpose | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Remote Desktop (RDP) Logon Detected** | EventID 4624 (LogonType 10) | `low` | Tracks interactive remote desktop logons. | T1021.001 (Lateral Movement) |
| **Terminal Services RDP Connection Established** | EventID 1149 (TerminalServices) | `low` | Tracks RDP network connection establishment. | T1021.001 (Lateral Movement) |
| **UAC Bypass Via Fodhelper** | EventID 4688 (`fodhelper`) | `high` | Detects elevation of privilege without UAC prompt. | T1548.002 (Privilege Escalation) |
| **UAC Bypass Via Event Viewer** | EventID 4688 (`eventvwr.exe`) | `low` | Detects registry hijack UAC bypass. | T1548.002 (Privilege Escalation) |
| **USB Storage Device Plugged In** | EventID 20001, 20003 | `low` | Tracks external removable media connections. | T1052.001 (Initial Access / Exfiltration) |
| **LSASS Memory Dump Attempt** | EventID 4688 (`lsass` + `dump`) | `critical` | Detects LSASS memory dump for credential theft. | T1003.001 (Credential Access) |
| **BITSAdmin File Transfer Execution** | EventID 4688 (`bitsadmin` + `/transfer`) | `medium` | Detects suspicious background file downloads. | T1197 (Defense Evasion) |
| **Network Share Object Added** | EventID 5142 | `low` | Tracks newly created network shares. | T1021.002 (Lateral Movement) |

---

### `linux-auth`
Linux authentication, SSH logins, and privilege escalation events.

| Rule Title | Keywords / Condition | Level | Security Purpose | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **SSH Failed Password Authentication** | `Failed password for` | `low` | Detects SSH password brute-force attempts. | T1110 (Credential Access) |
| **SSH Login Attempt With Invalid User** | `Failed password for invalid user` | `medium` | Detects username enumeration attempts. | T1110 (Credential Access) |
| **Direct SSH Root Login Accepted** | `Accepted password for root` / `Accepted publickey for root` | `medium` | Audits direct SSH root logins. | T1078.003 (Initial Access) |
| **Sudo Authentication Failure** | `authentication failure`, `incorrect password attempt` | `medium` | Detects failed sudo privilege escalation. | T1548.003 (Privilege Escalation) |
| **Sudoers Configuration File Modified** | `etc/sudoers`, `visudo` | `high` | Detects unauthorized privilege rule modification. | T1548.003 (Privilege Escalation) |
| **New Linux User Account Created** | `new user: name=`, `useradd` | `medium` | Detects local backdoor user creation. | T1136.001 (Persistence) |

---

### `linux-system`
Linux persistence, system tampering, and defense evasion.

| Rule Title | Keywords / Condition | Level | Security Purpose | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Cron Job Created or Modified** | `REPLACE (`, `crontab[`, `/etc/cron` | `low` | Detects persistent scheduled cron tasks. | T1053.003 (Persistence) |
| **Systemd Service Installed or Started** | `Created symlink /etc/systemd/system/`, `systemd: Started` | `low` | Detects systemd service additions. | T1543.002 (Persistence) |
| **Password File Modified or Password Changed** | `password changed for`, `/etc/shadow` | `low` | Audits password changes and shadow file tampering. | T1098 (Persistence) |
| **Linux Host Firewall Stopped or Flushed** | `ufw stop`, `firewalld: stopped`, `iptables -F` | `medium` | Detects host firewall disablement. | T1562.004 (Defense Evasion) |
| **Linux User Account Deleted** | `delete user`, `userdel` | `low` | Detects account deletion for sabotage or trail cleaning. | T1531 (Impact) |
| **New Linux Group Created** | `new group: name=`, `groupadd` | `low` | Detects suspicious new group additions. | T1136.001 (Persistence) |
| **Unsigned or Out-of-Tree Kernel Module Loaded** | `loading out-of-tree module`, `module verification failed` | `medium` | Detects rootkits or untrusted kernel modules. | T1547.006 (Persistence) |

---

### `network-threats`
Network appliances, UTM, and firewall logs (Fortinet, Cisco, Yamaha, Palo Alto).

| Rule Title | Keywords / Condition | Level | Security Purpose | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **VPN Authentication Failed** | `VPN authentication failed`, `IPsec authentication failed` | `medium` | Detects remote access VPN brute-force. | T1110 (Initial Access) |
| **FortiGate SSL VPN Authentication Failure** | `action=ssl-login-fail`, `SSL VPN login fail` | `medium` | Detects SSL-VPN login failures on FortiGate. | T1110 (Initial Access) |
| **Cisco Device Authentication Failure** | `%SEC_LOGIN-4-LOGIN_FAILED`, `%AAA-3-BADAUTH` | `medium` | Detects unauthorized logins on Cisco equipment. | T1110 (Credential Access) |
| **Yamaha Router Authentication Failure** | `Login failed`, `rejected TELNET`, `rejected SSH` | `medium` | Detects unauthorized access to Yamaha routers. | T1110 (Credential Access) |
| **Firewall Admin Console Login Failure** | `admin login failed`, `WebUI login failed` | `medium` | Detects administrative console intrusion attempts. | T1110 (Credential Access) |
| **Network Scan or Deny Flood** | `port scan`, `SYN flood`, `IP spoofing`, `LAND attack` | `high` | Detects reconnaissance scanning or DoS flood signs. | T1046 (Discovery) |
| **Palo Alto Networks Threat Detected** | `,THREAT,`, `,vulnerability,`, `,spyware,` | `high` | Alerts on known threats detected by PAN-OS NGFW. | T1190 (Initial Access) |
| **Network Device Configuration Changed** | `configuration changed`, `config commit`, `save config` | `low` | Audits network device configuration changes. | T1565 (Defense Evasion) |

---

### `web-attacks`
Web and proxy access logs (Apache, Nginx, reverse proxies).

| Rule Title | Keywords / Condition | Level | Security Purpose | MITRE ATT&CK |
| :--- | :--- | :---: | :--- | :--- |
| **Log4j JNDI Exploit Attempt (Log4Shell)** | `${jndi:ldap:`, `${jndi:rmi:`, `${jndi:dns:` | `critical` | Detects CVE-2021-44228 Log4Shell exploits. | T1190 (Initial Access) |
| **Path Traversal Attempt** | `/../`, `/etc/passwd`, `win.ini`, `%2e%2e%2f` | `high` | Detects directory traversal attacks. | T1190 (Initial Access) |
| **SQL Injection Attempt** | `UNION SELECT`, `' OR 1=1`, `WAITFOR DELAY` | `high` | Detects SQL injection payload attempts. | T1190 (Initial Access) |
| **WebShell Access or Execution** | `eval(base64_decode`, `c99.php`, `r57.php`, `wso.php` | `critical` | Detects access to installed webshells. | T1505.003 (Persistence) |
| **Web Vulnerability Scanner User-Agent** | `Nikto`, `sqlmap`, `gobuster`, `dirbuster`, `Acunetix` | `medium` | Detects automated vulnerability scan probes. | T1595.002 (Discovery) |
| **Cross Site Scripting (XSS) Pattern** | `<script>`, `javascript:alert(`, `<svg/onload=` | `medium` | Detects XSS injection payloads. | T1190 (Initial Access) |
| **Spring4Shell Remote Code Execution Attempt** | `class.module.classLoader` | `critical` | Detects CVE-2022-22965 Spring4Shell exploits. | T1190 (Initial Access) |
| **PHP Information Disclosure Access (phpinfo)** | `phpinfo.php`, `info.php`, `?phpinfo=` | `low` | Detects reconnaissance for phpinfo disclosure. | T1592.002 (Discovery) |

---

### `wazuh-compliance`
Compliance audit rules mapped to major standards.

| Rule Title | Event / Condition | Level | Compliance Standards |
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
Wazuh ruleset converted packs.

- **`wazuh-linux`** (6 rules): SSHD invalid user, brute-force (correlation: 6 times in 120s from same IP), root login, sudo command execution, unregistered sudoers execution, PAM authentication failures.
- **`wazuh-network`** (2 rules): Cisco admin login failures, FortiGate SSL-VPN brute-force (correlation supported).
- **`wazuh-web`** (2 rules): Web vulnerability scanner user-agents, sensitive hidden file probing (`.git`, `.env`, `.htpasswd`).

---

## 4. Custom Rules and Priority Override

You can combine external Sigma rules (`--rules`) with embedded rule packs.

### Priority Rule
When a rule has the same `id` (UUID), **the external custom rule takes precedence and automatically overrides the embedded rule**:

1. **`file:`** (High priority: custom rules specified via `--rules`)
2. **`pack:`** (Standard priority: embedded rule packs)

---

## 5. Wazuh Conversion Commands

### Convert Wazuh XML Rules to Sigma YAML (`convert-wazuh`)
```bash
# Convert a single Wazuh rule XML file to Sigma YAML
twsla sigma convert-wazuh -o ./converted-rules ./0095-sshd_rules.xml

# Batch convert a whole rules directory
twsla sigma convert-wazuh -o ./converted-rules /path/to/wazuh/rules/

# Output to stdout
twsla sigma convert-wazuh --stdout ./0095-sshd_rules.xml
```

### Convert Wazuh Decoders to Named Regexp (`convert-wazuh-decoder`)
```bash
twsla sigma convert-wazuh-decoder -o ./captures ./0310-ssh_decoders.xml
```

---

## 6. Timestamp-based Correlation Detection

Rules with `correlation:` metadata (`frequency`, `timeframe`, `group_by`) are evaluated using a sliding-window tracker:

- **Log Timestamp Precision**: Evaluated against historical `entry.Time` (nanoseconds), allowing accurate correlation replay over ingested logs.
- **Group Key Tracking**: Correlates events per unique group identifier (e.g. `client` IP).

---

## 7. Tag-based Aggregation and Charting in TUI

- **View Mode Toggle (`c` key)**: Cycle through **Events Table ⇄ Rule Count ⇄ Tag Count**.
- **Visual Charting (`h` / `g` keys)**: Generate HTML (ECharts) or terminal PNG/Sixel (NTCharts) bar charts of the top attack techniques and compliance violations.
- **Save Output (`s` key)**: Export active view mode as TSV table or chart images.
