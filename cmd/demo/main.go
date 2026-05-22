// cmd/demo -- cosmetic factory floor TUI.
//
// Prints a static but screenshot-ready view of what the forge dashboard
// looks like at runtime. No real data; all display logic only.
//
// Run: go run ./cmd/demo
package main

import (
	"fmt"
	"strings"
)

// ── Tokyo Night palette ───────────────────────────────────────────────────────

func fg(r, g, b int) string { return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b) }
func bg(r, g, b int) string { return fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b) }

const (
	reset = "\033[0m"
	bold  = "\033[1m"
)

var (
	cBg     = bg(26, 27, 38)    // #1a1b26  background
	cFg     = fg(192, 202, 245) // #c0caf5  default text
	cMuted  = fg(86, 95, 137)   // #565f89  dimmed / borders
	cBright = fg(169, 177, 214) // #a9b1d6  slightly brighter

	cBlue   = fg(122, 162, 247) // #7aa2f7
	cCyan   = fg(125, 207, 255) // #7dcfff
	cGreen  = fg(158, 206, 106) // #9ece6a
	cYellow = fg(224, 175, 104) // #e0af68
	cRed    = fg(247, 118, 142) // #f7768e
	cPurple = fg(187, 154, 247) // #bb9af7
	cOrange = fg(255, 158, 100) // #ff9e64
)

// ── Layout ────────────────────────────────────────────────────────────────────
//
//  total = 108  (including two outer ║)
//  inner = 106  (between outer ║)
//
//  three-column: ║ L(26) ║ M(52) ║ R(26) ║
//                1 + 26 + 1 + 52 + 1 + 26 + 1 = 108 ✓
//
//  hline:        ╠ ═(26) ╦ ═(52) ╦ ═(26) ╣  = 108 ✓

const (
	inner = 106
	lw    = 26
	mw    = 52
	rw    = 26
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// vis returns the visible (non-ANSI) rune count of s.
func vis(s string) int {
	esc := false
	n := 0
	for _, r := range s {
		if esc {
			if r == 'm' {
				esc = false
			}
			continue
		}
		if r == '\033' {
			esc = true
			continue
		}
		n++
	}
	return n
}

// pad pads s to exactly n visible runes.
func pad(s string, n int) string {
	v := vis(s)
	if v >= n {
		return s
	}
	return s + strings.Repeat(" ", n-v)
}

// col wraps color + reset + restore bg.
func col(color, s string) string { return color + s + reset + cBg }

// bar renders a progress bar: f filled cells out of total, w chars wide.
func bar(f, total, w int) string {
	filled := f * w / total
	empty := w - filled
	return cGreen + strings.Repeat("█", filled) +
		cMuted + strings.Repeat("░", empty) + reset + cBg
}

// ── Line builders ─────────────────────────────────────────────────────────────

func border(l, lsep, rsep, r string) string {
	return cBg + cMuted +
		l + strings.Repeat("═", lw) +
		lsep + strings.Repeat("═", mw) +
		rsep + strings.Repeat("═", rw) +
		r + reset
}

func top() string { return cBg + cMuted + "╔" + strings.Repeat("═", inner) + "╗" + reset }
func bot() string { return cBg + cMuted + "╚" + strings.Repeat("═", inner) + "╝" + reset }
func mid() string { return cBg + cMuted + "╠" + strings.Repeat("═", inner) + "╣" + reset }

func full(s string) string {
	return cBg + cMuted + "║" + reset + cBg + pad(s, inner) + cBg + cMuted + "║" + reset
}

func row(l, m, r string) string {
	sep := cBg + cMuted + "║" + reset
	return sep + cBg + pad(l, lw) + sep + cBg + pad(m, mw) + sep + cBg + pad(r, rw) + sep
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	p := fmt.Println

	// header
	p(top())
	leftH := "  " + col(bold+cBlue, "⚙  forge") + col(cMuted, "  ·  software factory")
	rightH := col(cGreen, "3 active") + col(cMuted, " · ") +
		col(cYellow, "12 queued") + col(cMuted, " · scan in ") +
		col(cCyan, "47m") + col(cMuted, " · ") + col(cGreen, "S1") + "  "
	gap := inner - vis(leftH) - vis(rightH)
	p(cBg + cMuted + "║" + reset + cBg + leftH + strings.Repeat(" ", gap) + rightH + cBg + cMuted + "║" + reset)

	// column titles
	p(border("╠", "╦", "╦", "╣"))
	p(row(
		"  "+col(bold+cBlue, "SIGNALS"),
		"  "+col(bold+cBlue, "ACTIVE AGENTS"),
		"  "+col(bold+cBlue, "REVIEW QUEUE"),
	))
	p(border("╠", "╬", "╬", "╣"))

	// ── block 1: capability gap / personal_finance / rename-account ──────────
	p(row(
		"  "+col(cRed, "●")+" "+col(cFg, "capability gap"),
		"  "+col(cCyan, "personal_finance"),
		"  "+col(cGreen, "✓")+" "+col(cFg, "rename-account-action"),
	))
	p(row(
		"    "+col(cMuted, ":rename-account"),
		"  "+col(cMuted, "›")+" "+col(cBright, "rename-account-action"),
		"    "+col(cGreen, "+87 lines")+" "+col(cMuted, "·")+" "+col(cGreen, "0 failures"),
	))
	p(row(
		"    "+col(cMuted, "7 users · 2d"),
		"  "+bar(14, 20, 18)+"  "+col(cOrange, "impl")+"  "+col(cMuted, "47 calls · 2m 14s"),
		"    "+col(cMuted, "49s")+"  "+col(cYellow, "[ Review ]"),
	))
	p(row(
		"",
		"  "+col(cMuted, "↳ Reading agent_actions.clj"),
		"",
	))

	// ── block 2: capability gap / my_api / unify-format-money ────────────────
	p(row(
		"  "+col(cRed, "●")+" "+col(cFg, "capability gap"),
		"  "+col(cCyan, "my_api"),
		"  "+col(cGreen, "✓")+" "+col(cFg, "unify-format-money"),
	))
	p(row(
		"    "+col(cMuted, ":set-budget-note"),
		"  "+col(cMuted, "›")+" "+col(cBright, "fix-null-ptr"),
		"    "+col(cGreen, "+36")+" "+col(cMuted, " / ")+col(cRed, "-83")+" "+col(cMuted, "lines"),
	))
	p(row(
		"    "+col(cMuted, "3 users · 6h"),
		"  "+bar(5, 20, 18)+"  "+col(cPurple, "draft")+" "+col(cMuted, "12 calls · 38s"),
		"    "+col(cMuted, "73s")+"  "+col(cYellow, "[ Review ]"),
	))
	p(row(
		"",
		"  "+col(cMuted, "↳ Writing spec..."),
		"",
	))

	// ── block 3: LIN ticket / company_api / fix-date-parsing ─────────────────
	p(row(
		"  "+col(cYellow, "◑")+" "+col(cFg, "LIN-4821"),
		"  "+col(cCyan, "company_api"),
		"  "+col(cGreen, "✓")+" "+col(cFg, "fix-date-parsing"),
	))
	p(row(
		"    "+col(cMuted, "null dereference"),
		"  "+col(cMuted, "›")+" "+col(cBright, "add-csv-export"),
		"    "+col(cGreen, "+12")+" "+col(cMuted, " / ")+col(cRed, "-3")+" "+col(cMuted, "lines"),
	))
	p(row(
		"    "+col(cMuted, "medium priority"),
		"  "+bar(2, 20, 18)+"  "+col(cOrange, "impl")+"  "+col(cMuted, " 9 calls · 21s"),
		"    "+col(cMuted, "31s")+"  "+col(cGreen, "Merged ✓"),
	))
	p(row(
		"",
		"  "+col(cMuted, "↳ Scanning export patterns..."),
		"",
	))

	// ── inline signal (no matching agent / review) ────────────────────────────
	p(row(
		"  "+col(cBlue, "○")+" "+col(cMuted, `"add CSV export"`),
		"",
		"",
	))
	p(row("", "", ""))
	p(row(
		"  "+col(cMuted, "[ + new task ]"),
		"",
		"",
	))

	// ── repo health strip ─────────────────────────────────────────────────────
	p(border("╠", "╩", "╩", "╣"))

	health := func(name, tests, gap, prs string) string {
		return "  " +
			pad(col(cCyan, name), 24) +
			col(cGreen, tests) +
			"   " + pad(col(cMuted, "last gap "+gap), 26) +
			col(cBright, prs)
	}
	p(full(health("personal_finance", "572 tests ✓", "2h ago", "4 PRs merged this week")))
	p(full(health("my_api          ", "214 tests ✓", "14m ago", "1 PR open · 0 failures")))
	p(full(health("company_api     ", " 89 tests ✓", "3d ago", "12 PRs merged this month")))

	// ── footer ────────────────────────────────────────────────────────────────
	p(mid())
	p(full("  " +
		col(cBlue, "⚙ forge") +
		col(cMuted, "  ·  3 agents running  ·  12 queued  ·  next scan in ") +
		col(cCyan, "47m") +
		col(cMuted, "  ·  uptime ") +
		col(cBright, "3h 12m") + "  "))
	p(bot())

	fmt.Print(reset)
}
