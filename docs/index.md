---
layout: default
title: autoclaude
description: Automatically resume Claude Code sessions after rate limits
---

# autoclaude

**Automatically resume [Claude Code](https://claude.ai/claude-code) sessions after rate limits.**

No more babysitting your Claude Code sessions. Run `autoclaude` in a tmux pane and it will automatically resume any rate-limited sessions in the same window.

## Install

```bash
brew install henryaj/tap/autoclaude
```

Or with Go:

```bash
go install github.com/henryaj/autoclaude@latest
```

## Usage

```bash
# Split your tmux window and run autoclaude
tmux split-window -h
autoclaude
```

That's it. When Claude Code hits a rate limit, autoclaude will wait for the reset time and automatically resume the session.

## How it works

1. Polls all tmux panes in the current window every 5 seconds
2. Detects rate limit messages (e.g., "Usage limit reached ∙ resets 3pm")
3. Waits until the limit resets, plus a small buffer
4. Sends keystrokes to resume the session

## Requirements

- tmux
- Claude Code sessions in other panes of the same window

## Links

- [GitHub](https://github.com/henryaj/autoclaude)
- [Releases](https://github.com/henryaj/autoclaude/releases)
