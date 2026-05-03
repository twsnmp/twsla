# TWSLA Test Expansion and VHS Recording Automation

## Problem Statement
We want to introduce unit tests and E2E tests to ensure the reliability of each command, and specifically automate MP4 recording using VHS to verify and demonstrate TUI operations using `bubbletea`.

## Recommended Direction
Separate the roles of unit tests (logic verification) and E2E tests (behavior and recording verification), and build a mechanism to easily execute recordings locally through `Taskfile`.

1.  **Unit Tests:** Expand existing `cmd/*_test.go`. Execute in GitHub Actions.
2.  **E2E/Recording Tests:** Use `vhs` (`.tape` files). Primarily for local execution.
3.  **Data Management:** Maintain `testlog/syslog.log` as the reference data.
4.  **Automation:** Add recording tasks to `Taskfile.yaml`.

## Key Assumptions to Validate
- [x] Can `bubbletea` animations and response speeds be captured stably with `vhs` `Sleep` settings?
- [x] Does the data volume in `testlog/syslog.log` match the recording time (is it appropriate for a demo)?
- [x] Installation status of `vhs`, `ffmpeg`, and `ttyd` in the development environment.

## MVP Scope
- Maintenance of `testlog/syslog.log`.
- Creation of `.tape` files for major commands (`anomaly`, `search`, `count`).
- Addition of `record` task to `Taskfile` (runs `vhs` and outputs videos to `images/`).
- Setup where `go test -short ./...` passes in GitHub Actions.

## Not Doing
- Recording of HTML report operations in the browser (terminal only for now).
- MP4 generation on GitHub Actions (due to resource consumption and tool dependencies, restricted to local only).
- Perfect E2E coverage of all commands (prioritize major TUI commands first).

## Open Questions
- Should the recorded MP4s be committed to the repository, or generated only during release? (Synchronizing with the PNGs in `images/`).
