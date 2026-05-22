// cmd/demo -- cosmetic factory floor TUI.
//
// Run:  go run ./cmd/demo
// Use a dark terminal, minimum 110 columns, monospace font.
package main

import (
	"fmt"
	"strings"
)

// ── Palette: Tokyo Night ──────────────────────────────────────────────────────

func fg(r, g, b int) string  { return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b) }
func bgc(r, g, b int) string { return fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b) }

const (
	// safeReset resets all SGR attributes then immediately restores the
	// background, preventing the terminal-default background from bleeding
	// through between adjacent colored segments.
	safeReset = "\033[0m\033[48;2;26;27;38m"
	bold      = "\033[1m"
	endAll    = "\033[0m"
)

var (
	// backgrounds
	cBg = bgc(26, 27, 38) // #1a1b26

	// foregrounds
	cFg     = fg(192, 202, 245) // #c0caf5  default text
	cBright = fg(169, 177, 214) // #a9b1d6
	cMuted  = fg(101, 112, 153) // #657199  metadata
	cDim    = fg(65, 72, 115)   // barely visible

	// Border: unambiguously cold blue — never pink.
	cBorder = fg(72, 92, 160) // #485ca0

	// accents
	cBlue   = fg(122, 162, 247) // #7aa2f7
	cCyan   = fg(125, 207, 255) // #7dcfff
	cGreen  = fg(158, 206, 106) // #9ece6a
	cYellow = fg(224, 175, 104) // #e0af68
	cRed    = fg(247, 118, 142) // #f7768e
	cPurple = fg(187, 154, 247) // #bb9af7
	cOrange = fg(255, 158, 100) // #ff9e64

	// progress bar shades
	cBarImpl  = fg(99, 146, 234)  // blue fill  — impl stage
	cBarDraft = fg(157, 124, 216) // purple fill — draft stage
	cBarEmpty = fg(38, 44, 72)    // nearly invisible empty
)

// ── Layout ────────────────────────────────────────────────────────────────────
//
//   total = 108 chars
//   inner = 106
//
//   ║ L(26) ║ M(52) ║ R(26) ║   →  1+26+1+52+1+26+1 = 108 ✓

const (
	inner = 106
	lw    = 26
	mw    = 52
	rw    = 26
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// vis counts visible (non-ANSI) display cells.
// ⚙ (U+2699) is 2 cells wide in most terminal fonts.
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
		if r == '⚙' {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func pad(s string, n int) string {
	v := vis(s)
	if v >= n {
		return s
	}
	return s + strings.Repeat(" ", n-v)
}

// c wraps s in a foreground color and uses safeReset to cleanly restore
// the background after — no bleeding into adjacent segments.
func c(color, s string) string { return color + s + safeReset }

// bar renders a progress bar of the given display width.
func bar(filled, total, width int, fillColor string) string {
	f := filled * width / total
	e := width - f
	return fillColor + strings.Repeat("█", f) +
		safeReset + cBarEmpty + strings.Repeat("░", e) + safeReset
}

// ── Line builders ─────────────────────────────────────────────────────────────

func hline(l, lsep, rsep, r string) string {
	return cBg + cBorder +
		l + strings.Repeat("═", lw) +
		lsep + strings.Repeat("═", mw) +
		rsep + strings.Repeat("═", rw) +
		r + safeReset
}

func topBar() string { return cBg + cBorder + "╔" + strings.Repeat("═", inner) + "╗" + safeReset }
func botBar() string { return cBg + cBorder + "╚" + strings.Repeat("═", inner) + "╝" + safeReset }
func midBar() string { return cBg + cBorder + "╠" + strings.Repeat("═", inner) + "╣" + safeReset }

func full(s string) string {
	return cBg + cBorder + "║" + safeReset +
		cBg + pad(s, inner) +
		cBg + cBorder + "║" + safeReset
}

func row(l, m, r string) string {
	sep := cBg + cBorder + "║" + safeReset
	return sep + cBg + pad(l, lw) +
		sep + cBg + pad(m, mw) +
		sep + cBg + pad(r, rw) +
		sep
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	p := fmt.Println

	// ── header ───────────────────────────────────────────────────────────────
	p(topBar())

	lh := "  " + c(bold+cBlue, "⚙  forge") + c(cDim, "  ·  ") + c(cMuted, "software factory")
	rh := c(cGreen, "3 active") + c(cDim, "  ·  ") +
		c(cYellow, "12 queued") + c(cDim, "  ·  scan in ") +
		c(cCyan, "47m") + c(cDim, "  ·  ") + c(bold+cGreen, "S1") + "  "
	p(cBg + cBorder + "║" + safeReset +
		cBg + lh + strings.Repeat(" ", inner-vis(lh)-vis(rh)) + rh +
		cBg + cBorder + "║" + safeReset)

	// ── column titles ─────────────────────────────────────────────────────────
	p(hline("╠", "╦", "╦", "╣"))
	p(row(
		"  "+c(bold+cBright, "SIGNALS"),
		"  "+c(bold+cBright, "ACTIVE AGENTS"),
		"  "+c(bold+cBright, "REVIEW QUEUE"),
	))
	p(hline("╠", "╬", "╬", "╣"))

	// ── agent 1 ───────────────────────────────────────────────────────────────
	p(row(
		"  "+c(cRed, "●")+"  "+c(cFg, "capability gap"),
		"  "+c(bold+cCyan, "personal_finance"),
		"  "+c(cGreen, "✓")+"  "+c(cFg, "rename-account"),     // 26: 2+1+2+14=19 ✓
	))
	p(row(
		"     "+c(cMuted, ":rename-account"),
		"     "+c(cDim, "›")+"  "+c(cBright, "rename-account-action"),
		"  "+c(cGreen, "+87")+" "+c(cMuted, "lines")+"  "+c(cGreen, "0 fails"), // 21 ✓
	))
	p(row(
		"     "+c(cMuted, "7 users  ·  2d ago"),
		"     "+c(cOrange, "impl")+"   "+bar(14, 20, 17, cBarImpl)+"  "+c(cMuted, "47 calls  2m 14s"),
		"  "+c(cMuted, "49s")+"   "+c(cYellow, "[ Review ]"),  // 17 ✓
	))
	p(row(
		"",
		"     "+c(cMuted, "↳ Reading agent_actions.clj"),
		"",
	))
	p(row("", "", ""))

	// ── agent 2 ───────────────────────────────────────────────────────────────
	p(row(
		"  "+c(cRed, "●")+"  "+c(cFg, "capability gap"),
		"  "+c(bold+cCyan, "my_api"),
		"  "+c(cGreen, "✓")+"  "+c(cFg, "unify-format-money"), // 22 ✓
	))
	p(row(
		"     "+c(cMuted, ":set-budget-note"),
		"     "+c(cDim, "›")+"  "+c(cBright, "fix-null-ptr"),
		"  "+c(cGreen, "+36")+" "+c(cMuted, "/")+" "+c(cRed, "-83")+" "+c(cMuted, "lines"), // 19 ✓
	))
	p(row(
		"     "+c(cMuted, "3 users  ·  6h ago"),
		"     "+c(cPurple, "draft")+"  "+bar(5, 20, 17, cBarDraft)+"  "+c(cMuted, "12 calls  38s"),
		"  "+c(cMuted, "73s")+"   "+c(cYellow, "[ Review ]"),  // 17 ✓
	))
	p(row(
		"",
		"     "+c(cMuted, "↳ Writing spec..."),
		"",
	))
	p(row("", "", ""))

	// ── agent 3 ───────────────────────────────────────────────────────────────
	p(row(
		"  "+c(cYellow, "◑")+"  "+c(cFg, "LIN-4821"),
		"  "+c(bold+cCyan, "company_api"),
		"  "+c(cGreen, "✓")+"  "+c(cFg, "fix-date-parsing"),   // 20 ✓
	))
	p(row(
		"     "+c(cMuted, "null dereference"),
		"     "+c(cDim, "›")+"  "+c(cBright, "add-csv-export"),
		"  "+c(cGreen, "+12")+" "+c(cMuted, "/")+" "+c(cRed, "-3")+" "+c(cMuted, "lines"), // 18 ✓
	))
	p(row(
		"     "+c(cMuted, "medium priority"),
		"     "+c(cOrange, "impl")+"   "+bar(2, 20, 17, cBarImpl)+"  "+c(cMuted, " 9 calls  21s"),
		"  "+c(cMuted, "31s")+"   "+c(cGreen, "Merged ✓"),     // 15 ✓
	))
	p(row(
		"",
		"     "+c(cMuted, "↳ Scanning export patterns..."),
		"",
	))

	// ── inline + new task ────────────────────────────────────────────────────
	p(row(
		"  "+c(cBlue, "○")+"  "+c(cMuted, `"add CSV export"`),
		"",
		"",
	))
	p(row("", "", ""))
	p(row(
		"  "+c(cDim, "[ + new task ]"),
		"",
		"",
	))

	// ── repo health ───────────────────────────────────────────────────────────
	p(hline("╠", "╩", "╩", "╣"))

	health := func(repo, tests, since, prs string) string {
		return "  " + pad(c(cCyan, repo), 24) +
			c(cGreen, tests) + "   " +
			pad(c(cMuted, "last gap  "+since), 28) +
			c(cBright, prs)
	}
	p(full(health("personal_finance", "572 tests ✓", "2h ago  ", "4 PRs merged this week")))
	p(full(health("my_api          ", "214 tests ✓", "14m ago ", "1 PR open  ·  0 failures")))
	p(full(health("company_api     ", " 89 tests ✓", "3d ago  ", "12 PRs merged this month")))

	// ── footer ────────────────────────────────────────────────────────────────
	p(midBar())
	p(full("  " +
		c(cBlue, "⚙ forge") +
		c(cDim, "  ·  3 agents running  ·  12 queued  ·  next scan in ") +
		c(cCyan, "47m") +
		c(cDim, "  ·  uptime ") +
		c(cBright, "3h 12m") + "  "))
	p(botBar())

	fmt.Print(endAll)
}
