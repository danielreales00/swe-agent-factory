package bridge

import (
	"fmt"
	"strings"
)

// Command identifies which slash command the user typed. Cmd_None means
// the message wasn't a slash command and should fall through to pi.
type CommandKind int

const (
	CmdNone CommandKind = iota
	CmdRepo
	CmdCancel
	CmdUnknown
)

// Command is the parsed result of a single user message.
type Command struct {
	Kind CommandKind
	Arg  string // for /repo <name>; "" otherwise
}

// ParseCommand recognizes /repo, /repo <name>, /cancel. Anything else
// starting with `/` is CmdUnknown; non-slash messages are CmdNone.
// Whitespace around the message is trimmed; the command is lowercased
// but the argument is preserved verbatim.
func ParseCommand(text string) Command {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "/") {
		return Command{Kind: CmdNone}
	}
	head, rest, _ := strings.Cut(t[1:], " ")
	arg := strings.TrimSpace(rest)
	switch strings.ToLower(head) {
	case "repo":
		return Command{Kind: CmdRepo, Arg: arg}
	case "cancel":
		return Command{Kind: CmdCancel}
	default:
		return Command{Kind: CmdUnknown, Arg: head}
	}
}

// CommandReply is the chat-side outcome of executing a command. The
// Handler sends Reply (always) and then optionally applies Effect (cancel
// pi, switch target).
type CommandReply struct {
	Reply        string
	SwitchTarget *Target // set by /repo <name> when valid + pi idle
	Cancel       bool    // set by /cancel when pi is alive
}

// ExecuteCommand applies pure rules: it inspects the session state and the
// registry, decides what reply to send, and returns Effect flags for the
// handler to apply. Doesn't mutate the session itself.
func ExecuteCommand(cmd Command, sess *Session, reg *Registry) CommandReply {
	switch cmd.Kind {
	case CmdRepo:
		if cmd.Arg == "" {
			return CommandReply{Reply: repoStatusLine(sess, reg)}
		}
		t, ok := reg.Get(cmd.Arg)
		if !ok {
			return CommandReply{Reply: fmt.Sprintf("❌ unknown target %q. %s",
				cmd.Arg, listLine(reg))}
		}
		if sess.Pi != nil {
			return CommandReply{Reply: "⏳ pi is running. /cancel first, then /repo " + cmd.Arg + "."}
		}
		if sess.Target.Name == t.Name {
			return CommandReply{Reply: "already on " + t.Name + "."}
		}
		return CommandReply{
			Reply:        "✅ switched to " + t.Name + ".",
			SwitchTarget: &t,
		}

	case CmdCancel:
		if sess.Pi == nil {
			return CommandReply{Reply: "no pi running."}
		}
		return CommandReply{
			Reply:  "✋ cancelling pi…",
			Cancel: true,
		}

	case CmdUnknown:
		return CommandReply{Reply: "❓ unknown command /" + cmd.Arg + ". known: /repo, /cancel."}
	}
	// CmdNone shouldn't reach here; caller is expected to route around it.
	return CommandReply{}
}

func repoStatusLine(sess *Session, reg *Registry) string {
	current := sess.Target.Name
	if current == "" {
		current = "(none)"
	}
	return fmt.Sprintf("📁 current: %s\n%s", current, listLine(reg))
}

func listLine(reg *Registry) string {
	return "available: [" + strings.Join(reg.Names(), ", ") + "]"
}
