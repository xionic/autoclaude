# autoclaude

Automatically resume [Claude Code](https://claude.ai/claude-code) sessions when you hit your rate limit.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install henryaj/tap/autoclaude
```

### Go install

```bash
go install github.com/henryaj/autoclaude@latest
```

### Download binary

Download from [GitHub Releases](https://github.com/henryaj/autoclaude/releases).

## Usage

Run `autoclaude` in a tmux pane alongside your Claude Code sessions:

```bash
# Split your tmux window
tmux split-window -h

# Run autoclaude in the new pane
autoclaude
```

That's it. When Claude Code hits a rate limit in any other pane in the same window, autoclaude will:

1. Detect the rate limit message
2. Wait until the limit resets
3. Automatically resume the session

### Options

```
-v          Enable verbose/debug logging
-version    Show version information
-test       Test mode: wait 10s then send resume sequence
```

## How it works

autoclaude monitors all tmux panes in the current window (except the one it's running in) by polling every 5 seconds. When it detects a rate limit message like:

```
Usage limit reached ∙ resets 3pm
```

It parses the reset time, waits until then (plus a random 5-10 second buffer), then sends keystrokes to resume the session:

1. `Enter` - dismisses any selector menu
2. `continue` + `Enter` - resumes the Claude Code session

## Requirements

- Must be run inside a tmux session
- Claude Code sessions must be in other panes of the same tmux window

## License

MIT
