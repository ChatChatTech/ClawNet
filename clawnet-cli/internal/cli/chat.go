package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/config"
	"golang.org/x/term"
)

// ── Entry point ────────────────────────────────────────────────────

func cmdChat() error {
	args := os.Args[2:]

	// Help
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		fmt.Println(`Usage: clawnet chat [peer_id message]

Full-screen mail TUI (default):
  clawnet chat              Open interactive chat

Quick send:
  clawnet chat <peer_id> <message>

TUI Controls:
  Tab / ←→       Switch panels (Inbox, Thread, Input)
  ↑↓             Select inbox / scroll thread
  Enter           Open thread / send message
  F1              New conversation
  F2              Refresh
  F3              Delete selected conversation
  Esc / Ctrl+C    Quit
  quit()          Type in input to exit`)
		return nil
	}

	// Quick-send: clawnet chat <peer_id> <message>
	if len(args) >= 2 {
		peerID := args[0]
		body := strings.Join(args[1:], " ")
		return chatQuickSend(peerID, body)
	}

	return cmdChatTUI()
}

func chatQuickSend(peerID, body string) error {
	cfg, _ := config.Load()
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)
	if err := sendChatMsg(base, peerID, body); err != nil {
		fmt.Fprintf(os.Stderr, "\033[91m✗ Send error: %v\033[0m\n", err)
		return err
	}
	short := peerID
	if len(short) > 16 {
		short = short[:16] + "..."
	}
	fmt.Printf("\033[92m✓\033[0m Sent to %s\n", short)
	return nil
}

// ── TUI panels ─────────────────────────────────────────────────────

const (
	chatPanelInbox  = 0
	chatPanelThread = 1
	chatPanelInput  = 2
	chatPanelCount  = 3
)

type inboxEntry struct {
	PeerID    string `json:"peer_id"`
	Direction string `json:"direction"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	Read      bool   `json:"read"`
}

type threadMsg struct {
	Direction string `json:"direction"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type peerReputation struct {
	PeerID         string  `json:"peer_id"`
	Score          float64 `json:"score"`
	TasksCompleted int     `json:"tasks_completed"`
	Contributions  int     `json:"contributions"`
}

type chatTUI struct {
	base         string
	panel        int
	inbox        []inboxEntry
	selectedIdx  int
	inboxScroll  int
	thread       []threadMsg
	threadScroll int
	inputBuf     []rune
	inputCursor  int
	inputScroll  int
	statusMsg    string
	statusTime   time.Time
	newMode      bool
	newPeerBuf   []rune
	peerRep      map[string]*peerReputation // cached reputations

	// Lobster animation
	lobsterPos   int  // position along border perimeter
	lobsterShow  bool // visible or hidden
	lobsterTimer int  // frames until state change
	lobsterFrame int  // frame counter
	lobsterSpeed int  // frames per movement step (higher = slower)

	wantExit bool // signal main loop to return cleanly
}

func (s *chatTUI) selectedPeerID() string {
	if s.selectedIdx >= 0 && s.selectedIdx < len(s.inbox) {
		return s.inbox[s.selectedIdx].PeerID
	}
	return ""
}

func (s *chatTUI) setStatus(msg string) {
	s.statusMsg = msg
	s.statusTime = time.Now()
}

// lobsterLevel returns a rank title based on reputation score.
func lobsterLevel(score float64) (string, string) {
	switch {
	case score >= 100:
		return "🦞", "\033[1;38;2;255;220;50m" // gold
	case score >= 70:
		return "🦞", "\033[38;2;247;127;0m" // orange
	case score >= 50:
		return "🦐", "\033[38;2;230;57;70m" // red
	case score >= 30:
		return "🦐", "\033[38;2;140;30;35m" // dim red
	default:
		return "🦐", "\033[38;2;100;100;100m" // gray
	}
}

func lobsterRankName(score float64) string {
	switch {
	case score >= 100:
		return "Lobster King"
	case score >= 70:
		return "Lobster"
	case score >= 50:
		return "Shrimp"
	case score >= 30:
		return "Krill"
	default:
		return "Plankton"
	}
}

// ── Main TUI loop ──────────────────────────────────────────────────

func cmdChatTUI() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebUIPort)

	if _, err := http.Get(base + "/api/dm/inbox"); err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("not a terminal")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot enter raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	fmt.Print("\033[?1049h")
	defer fmt.Print("\033[?25h\033[?1049l")

	state := &chatTUI{
		base:    base,
		panel:   chatPanelInbox,
		peerRep: make(map[string]*peerReputation),
	}

	state.inbox = fetchInbox(base)
	if len(state.inbox) > 0 {
		pid := state.inbox[0].PeerID
		state.thread = fetchThreadMsgs(base, pid)
		state.fetchRepIfNeeded(pid)
	}

	// Lobster init
	state.lobsterShow = true
	state.lobsterTimer = 20 + rand.Intn(30)
	state.lobsterSpeed = 3 + rand.Intn(4) // 3..6 frames per step

	// UTF-8 aware input reader
	rawCh := make(chan []byte, 64)
	go func() {
		buf := make([]byte, 128)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				continue
			}
			b := make([]byte, n)
			copy(b, buf[:n])
			rawCh <- b
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	notifyResize(sigCh)
	defer signal.Stop(sigCh)

	renderTicker := time.NewTicker(150 * time.Millisecond)
	defer renderTicker.Stop()
	refreshTicker := time.NewTicker(3 * time.Second)
	defer refreshTicker.Stop()

	needRedraw := false
	_ = needRedraw
	prevSelectedPeer := ""

	for {
	drainInput:
		for {
			select {
			case raw := <-rawCh:
				needRedraw = true
				if len(raw) == 1 {
					state.handleKey(raw[0], &prevSelectedPeer)
				} else if len(raw) >= 3 && raw[0] == 27 && raw[1] == '[' {
					state.handleEsc(string(raw[2:]), &prevSelectedPeer)
				} else if len(raw) >= 3 && raw[0] == 27 && raw[1] == 'O' {
					// ESC O P / ESC O Q etc (F-keys)
					state.handleEsc("O"+string(raw[2:]), &prevSelectedPeer)
				} else if len(raw) >= 1 && raw[0] == 27 {
					// Bare Esc or Esc+unknown — exit
					state.wantExit = true
				} else if state.panel == chatPanelInput {
					// Multi-byte UTF-8 input (Chinese etc)
					state.insertUTF8(raw)
				}
			default:
				break drainInput
			}
		}

		if state.wantExit {
			return nil
		}

		select {
		case <-sigCh:
			needRedraw = true
		case <-refreshTicker.C:
			state.inbox = fetchInbox(base)
			if pid := state.selectedPeerID(); pid != "" {
				state.thread = fetchThreadMsgs(base, pid)
				state.fetchRepIfNeeded(pid)
			}
			needRedraw = true
		case <-renderTicker.C:
			// Advance lobster animation
			state.lobsterFrame++
			if state.lobsterShow && state.lobsterFrame%state.lobsterSpeed == 0 {
				state.lobsterPos++
			}
			state.lobsterTimer--
			if state.lobsterTimer <= 0 {
				state.lobsterShow = !state.lobsterShow
				if state.lobsterShow {
					state.lobsterTimer = 30 + rand.Intn(50)
					state.lobsterSpeed = 3 + rand.Intn(4) // 3..6 frames per step
				} else {
					state.lobsterTimer = 20 + rand.Intn(35)
				}
			}
			needRedraw = true

			if state.statusMsg != "" && time.Since(state.statusTime) > 3*time.Second {
				state.statusMsg = ""
			}
		}

		w, h, err := term.GetSize(fd)
		if err != nil {
			w, h = 80, 24
		}
		frame, curRow, curCol := renderChatFrame(state, w, h)
		cursorPos := fmt.Sprintf("\033[%d;%dH\033[?25h", curRow, curCol)
		fmt.Print("\033[?2026h\033[H" + frame + cursorPos + "\033[?2026l")
		needRedraw = false
	}
}

// ── Input handling ─────────────────────────────────────────────────

func (s *chatTUI) insertUTF8(raw []byte) {
	// Decode all runes from raw bytes
	for len(raw) > 0 {
		r, size := utf8.DecodeRune(raw)
		if r == utf8.RuneError && size <= 1 {
			break
		}
		if s.newMode {
			s.newPeerBuf = append(s.newPeerBuf, r)
		} else {
			s.inputBuf = append(s.inputBuf[:s.inputCursor], append([]rune{r}, s.inputBuf[s.inputCursor:]...)...)
			s.inputCursor++
		}
		raw = raw[size:]
	}
}

func (s *chatTUI) handleKey(key byte, prevPeer *string) {
	// Esc — exit from any panel
	if key == 27 {
		s.wantExit = true
		return
	}
	// Ctrl+C exits from anywhere
	if key == 3 {
		s.wantExit = true
		return
	}

	if s.panel == chatPanelInput {
		switch key {
		case 13: // Enter — send
			if s.newMode {
				pid := strings.TrimSpace(string(s.newPeerBuf))
				if pid != "" {
					found := false
					for i, e := range s.inbox {
						if e.PeerID == pid {
							s.selectedIdx = i
							found = true
							break
						}
					}
					if !found {
						s.inbox = append([]inboxEntry{{PeerID: pid, Body: "(new)", CreatedAt: time.Now().Format(time.RFC3339)}}, s.inbox...)
						s.selectedIdx = 0
					}
					s.thread = fetchThreadMsgs(s.base, pid)
					s.threadScroll = 0
					s.fetchRepIfNeeded(pid)
				}
				s.newMode = false
				s.newPeerBuf = nil
				s.inputBuf = nil
				s.inputCursor = 0
			} else {
				text := strings.TrimSpace(string(s.inputBuf))
				if text == "exit()" || text == "quit()" {
					s.wantExit = true
					return
				}
				if text != "" && s.selectedPeerID() != "" {
					if err := sendChatMsg(s.base, s.selectedPeerID(), text); err != nil {
						s.setStatus("✗ " + err.Error())
					} else {
						s.setStatus("✓ Sent")
						s.thread = fetchThreadMsgs(s.base, s.selectedPeerID())
						s.threadScroll = 0
					}
				}
				s.inputBuf = nil
				s.inputCursor = 0
			}
		case 127, 8: // Backspace
			if s.newMode {
				if len(s.newPeerBuf) > 0 {
					s.newPeerBuf = s.newPeerBuf[:len(s.newPeerBuf)-1]
				}
			} else {
				if s.inputCursor > 0 {
					s.inputBuf = append(s.inputBuf[:s.inputCursor-1], s.inputBuf[s.inputCursor:]...)
					s.inputCursor--
				}
			}
		case '\t': // Tab — next panel
			s.panel = (s.panel + 1) % chatPanelCount
		default:
			if key >= 32 && key < 127 {
				if s.newMode {
					s.newPeerBuf = append(s.newPeerBuf, rune(key))
				} else {
					s.inputBuf = append(s.inputBuf[:s.inputCursor], append([]rune{rune(key)}, s.inputBuf[s.inputCursor:]...)...)
					s.inputCursor++
				}
			}
		}
	} else {
		switch key {
		case '\t': // Tab — next panel
			s.panel = (s.panel + 1) % chatPanelCount
		case 13: // Enter
			if s.panel == chatPanelInbox && len(s.inbox) > 0 {
				s.panel = chatPanelThread
				pid := s.selectedPeerID()
				if pid != *prevPeer {
					s.thread = fetchThreadMsgs(s.base, pid)
					s.threadScroll = 0
					s.fetchRepIfNeeded(pid)
					*prevPeer = pid
				}
			}
		}
	}
}

func (s *chatTUI) handleEsc(seq string, prevPeer *string) {
	switch seq {
	case "A": // Up
		if s.panel == chatPanelInbox {
			if s.selectedIdx > 0 {
				s.selectedIdx--
				pid := s.selectedPeerID()
				if pid != *prevPeer {
					s.thread = fetchThreadMsgs(s.base, pid)
					s.threadScroll = 0
					s.fetchRepIfNeeded(pid)
					*prevPeer = pid
				}
			}
			if s.selectedIdx < s.inboxScroll {
				s.inboxScroll = s.selectedIdx
			}
		} else if s.panel == chatPanelThread {
			s.threadScroll++
		}
	case "B": // Down
		if s.panel == chatPanelInbox {
			if s.selectedIdx < len(s.inbox)-1 {
				s.selectedIdx++
				pid := s.selectedPeerID()
				if pid != *prevPeer {
					s.thread = fetchThreadMsgs(s.base, pid)
					s.threadScroll = 0
					s.fetchRepIfNeeded(pid)
					*prevPeer = pid
				}
			}
		} else if s.panel == chatPanelThread {
			if s.threadScroll > 0 {
				s.threadScroll--
			}
		}
	case "C": // Right — next panel
		if s.panel == chatPanelInput && s.inputCursor < len(s.inputBuf) {
			s.inputCursor++
		} else if s.panel != chatPanelInput {
			s.panel = (s.panel + 1) % chatPanelCount
		}
	case "D": // Left — prev panel
		if s.panel == chatPanelInput && s.inputCursor > 0 {
			s.inputCursor--
		} else if s.panel != chatPanelInput {
			s.panel = (s.panel - 1 + chatPanelCount) % chatPanelCount
		}
	case "Z": // Shift+Tab — prev panel
		s.panel = (s.panel - 1 + chatPanelCount) % chatPanelCount
	// F1 = new chat, F2 = refresh, F5 = quit (ESC O P / ESC [ 1 1 ~)
	case "11~", "OP": // F1 — new chat
		s.newMode = true
		s.newPeerBuf = nil
		s.panel = chatPanelInput
	case "12~", "OQ": // F2 — refresh
		s.inbox = fetchInbox(s.base)
		if pid := s.selectedPeerID(); pid != "" {
			s.thread = fetchThreadMsgs(s.base, pid)
		}
		s.setStatus("Refreshed")
	case "15~": // F5 — quit
		s.wantExit = true
	case "13~", "OR": // F3 — delete selected conversation
		if s.selectedPeerID() != "" {
			if err := deleteInbox(s.base, s.selectedPeerID()); err != nil {
				s.setStatus("✗ Delete: " + err.Error())
			} else {
				s.inbox = fetchInbox(s.base)
				if s.selectedIdx >= len(s.inbox) {
					s.selectedIdx = len(s.inbox) - 1
				}
				if s.selectedIdx < 0 {
					s.selectedIdx = 0
				}
				s.thread = nil
				s.threadScroll = 0
				if pid := s.selectedPeerID(); pid != "" {
					s.thread = fetchThreadMsgs(s.base, pid)
				}
				s.setStatus("✓ Deleted")
			}
		}
	}
}

func (s *chatTUI) fetchRepIfNeeded(peerID string) {
	if _, ok := s.peerRep[peerID]; ok {
		return
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/reputation/%s", s.base, peerID))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var rep peerReputation
	if json.NewDecoder(resp.Body).Decode(&rep) == nil {
		s.peerRep[peerID] = &rep
	}
}

// ── Rendering ──────────────────────────────────────────────────────

func renderChatFrame(s *chatTUI, termW, termH int) (string, int, int) {
	innerW := termW - 2
	if innerW < 20 {
		innerW = 20
	}

	// Layout: header(1) + colHeaders(1) + colSep(1) + bodyRows + inputSep(1) + inputRows(3) + helpSep(1) + help(1) + bottom(1)
	inputH := 3
	overhead := 8 // header + colhdr + colsep + inputsep + 3*input + helpsep + help + bottom = 1+1+1+1+3+1+1+1 = 10 but input counts
	bodyH := termH - overhead - inputH
	if bodyH < 3 {
		bodyH = 3
	}

	// Left panel width
	leftW := innerW * 2 / 5
	if leftW < 22 {
		leftW = 22
	}
	if leftW > 42 {
		leftW = 42
	}
	rightW := innerW - leftW - 1
	if rightW < 20 {
		rightW = 20
		leftW = innerW - rightW - 1
	}

	// Compute border perimeter for lobster (top + bottom only)
	perimeter := 2 * innerW

	var sb strings.Builder

	// ── Header (top border) ──
	titleText := " ClawNet Mail "
	unread := 0
	for _, e := range s.inbox {
		if !e.Read && e.Direction == "received" {
			unread++
		}
	}
	countText := ""
	if unread > 0 {
		countText = fmt.Sprintf(" %d unread ", unread)
	}
	headerDisplay := titleText + countText
	headerLen := visibleLen(headerDisplay)
	fillTotal := innerW - headerLen
	if fillTotal < 2 {
		fillTotal = 2
	}
	dashL := fillTotal / 2
	dashR := fillTotal - dashL

	// Check if lobster is on top border
	topBorderStr := buildBorderWithLobster(s, "top", innerW, perimeter, dashL, dashR, headerDisplay, unread > 0, countText, titleText)
	sb.WriteString(topBorderStr)

	// ── Column headers ──
	inboxHL := s.panel == chatPanelInbox
	threadHL := s.panel == chatPanelThread

	leftHdr := " Inbox"
	rightHdr := " Thread"
	if pid := s.selectedPeerID(); pid != "" {
		short := pid
		if len(short) > 16 {
			short = short[:16]
		}
		rep := s.peerRep[pid]
		if rep != nil {
			emoji, color := lobsterLevel(rep.Score)
			rightHdr = fmt.Sprintf(" %s %s%s%s %s", emoji, color, lobsterRankName(rep.Score), cReset, short)
		} else {
			rightHdr = " " + short
		}
	}

	sb.WriteString(cBorder + "│" + cReset)
	if inboxHL {
		sb.WriteString(cHighlight)
	} else {
		sb.WriteString(cSelfInfo)
	}
	sb.WriteString(padRight(leftHdr, leftW))
	sb.WriteString(cReset)
	sb.WriteString(cBorder + "│" + cReset)
	if threadHL {
		sb.WriteString(cHighlight)
	} else {
		sb.WriteString(cPeerInfo)
	}
	sb.WriteString(padRight(rightHdr, rightW))
	sb.WriteString(cReset)
	sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")

	// ── Separator under headers ──
	sb.WriteString(cBorder + "├")
	if inboxHL {
		sb.WriteString(cHighlight + strings.Repeat("━", leftW) + cReset + cBorder)
	} else {
		sb.WriteString(strings.Repeat("─", leftW))
	}
	sb.WriteString("┼")
	if threadHL {
		sb.WriteString(cHighlight + strings.Repeat("━", rightW) + cReset + cBorder)
	} else {
		sb.WriteString(strings.Repeat("─", rightW))
	}
	sb.WriteString("┤" + cReset + "\033[K\r\n")

	// ── Body rows ──
	inboxLines := renderInboxLines(s, leftW, bodyH)
	threadLines := renderThreadLines(s, rightW, bodyH)

	for i := 0; i < bodyH; i++ {
		leftContent := strings.Repeat(" ", leftW)
		rightContent := strings.Repeat(" ", rightW)
		if i < len(inboxLines) {
			leftContent = inboxLines[i]
		}
		if i < len(threadLines) {
			rightContent = threadLines[i]
		}

		sb.WriteString(cBorder + "│" + cReset)
		sb.WriteString(leftContent)
		sb.WriteString(cReset)
		sb.WriteString(cBorder + "│" + cReset)
		sb.WriteString(rightContent)
		sb.WriteString(cReset)
		sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")
	}

	// ── Separator above input ──
	inputHL := s.panel == chatPanelInput
	sb.WriteString(cBorder + "├")
	if inputHL {
		sb.WriteString(cHighlight + strings.Repeat("━", innerW) + cReset + cBorder)
	} else {
		sb.WriteString(strings.Repeat("─", innerW))
	}
	sb.WriteString("┤" + cReset + "\033[K\r\n")

	// ── Input rows (3 lines) ──
	inputLines := renderInputLines(s, innerW, inputH)
	for _, line := range inputLines {
		sb.WriteString(cBorder + "│" + cReset)
		sb.WriteString(line)
		vl := visibleLen(line)
		if vl < innerW {
			sb.WriteString(strings.Repeat(" ", innerW-vl))
		}
		sb.WriteString(cBorder + "│" + cReset + "\033[K\r\n")
	}

	// ── Separator above help ──
	sb.WriteString(cBorder + "├")
	sb.WriteString(strings.Repeat("─", innerW))
	sb.WriteString("┤" + cReset + "\033[K\r\n")

	// ── Help line ──
	panelNames := []string{"Inbox", "Thread", "Input"}
	var tabText string
	if s.panel == chatPanelInput {
		tabText = "\033[1;33m" + "Tab:Panel" + cReset + cHelp
	} else {
		tabText = "Tab:Panel"
	}
	help := fmt.Sprintf(" %s [%s]", tabText, panelNames[s.panel])
	if s.panel == chatPanelInbox {
		help += "  ↑↓:Select  Enter:Open"
	} else if s.panel == chatPanelThread {
		help += "  ↑↓:Scroll"
	} else {
		help += "  Enter:Send  quit():Exit"
	}
	help += "  F1:New  F2:Refresh  F3:Delete  Esc:Quit"
	if s.statusMsg != "" {
		help += "  " + s.statusMsg
	}
	emitRow(&sb, cHelp+help+cReset, innerW)

	// ── Bottom border ──
	bottomStr := buildBottomBorder(s, innerW, perimeter, termH)
	sb.WriteString(bottomStr)

	// Compute cursor position for active panel (1-based row, col)
	var curRow, curCol int
	switch s.panel {
	case chatPanelInbox:
		curRow = 4 + (s.selectedIdx - s.inboxScroll) // body starts at row 4
		curCol = 3
	case chatPanelThread:
		curRow = 4
		curCol = leftW + 4
	case chatPanelInput:
		inputRow := 3 + bodyH + 2 // inputSep + 1
		curRow = inputRow
		lineW := innerW - 3
		if lineW < 10 {
			lineW = 10
		}
		if s.newMode {
			curCol = 7 + len(s.newPeerBuf) // " To: " = 5 + border col
		} else {
			effCursor := s.inputCursor
			curRow += effCursor / lineW
			curCol = 2 + 3 + (effCursor % lineW) // border + prompt
		}
	}
	if curRow < 1 {
		curRow = 1
	}
	if curCol < 1 {
		curCol = 1
	}

	return sb.String(), curRow, curCol
}

// ── Lobster border helpers ─────────────────────────────────────────

func buildBorderWithLobster(s *chatTUI, side string, innerW, perimeter, dashL, dashR int, headerDisplay string, hasUnread bool, countText, titleText string) string {
	var sb strings.Builder
	// Top border: ┌───title───┐
	// Lobster position 0..innerW-1 = top border
	lobsterTop := -1
	if s.lobsterShow {
		pos := s.lobsterPos % perimeter
		if pos >= 0 && pos < innerW {
			lobsterTop = pos
		}
	}

	sb.WriteString(cBorder + "┌")
	topBar := make([]rune, innerW)
	for i := range topBar {
		topBar[i] = '─'
	}
	// Place title text
	titleStart := dashL
	titleRunes := []rune(titleText)
	for i, r := range titleRunes {
		if titleStart+i < innerW {
			topBar[titleStart+i] = r
		}
	}
	if hasUnread {
		countRunes := []rune(countText)
		countStart := titleStart + len(titleRunes)
		for i, r := range countRunes {
			if countStart+i < innerW {
				topBar[countStart+i] = r
			}
		}
	}

	// Render the top bar char by char
	titleEnd := titleStart + len(titleRunes)
	countStart := titleEnd
	countEnd := countStart
	if hasUnread {
		countEnd = countStart + len([]rune(countText))
	}

	for i := 0; i < innerW; i++ {
		if lobsterTop == i && i+1 < innerW {
			sb.WriteString(cReset + "🦞" + cBorder)
			i++ // 🦞 is 2 columns wide — skip next position
			continue
		}
		if i >= titleStart && i < titleEnd {
			if i == titleStart {
				sb.WriteString(cTitle)
			}
			sb.WriteRune(topBar[i])
			if i == titleEnd-1 {
				if hasUnread {
					sb.WriteString("\033[1;92m")
				} else {
					sb.WriteString(cReset + cBorder)
				}
			}
		} else if hasUnread && i >= countStart && i < countEnd {
			sb.WriteRune(topBar[i])
			if i == countEnd-1 {
				sb.WriteString(cReset + cBorder)
			}
		} else {
			sb.WriteRune('─')
		}
	}
	sb.WriteString("┐" + cReset + "\033[K\r\n")
	return sb.String()
}

func buildBottomBorder(s *chatTUI, innerW, perimeter, termH int) string {
	var sb strings.Builder
	// Bottom border: positions innerW..2*innerW-1
	lobsterBot := -1
	if s.lobsterShow {
		pos := s.lobsterPos % perimeter
		if pos >= innerW && pos < 2*innerW {
			lobsterBot = pos - innerW
		}
	}

	sb.WriteString(cBorder + "└")
	for i := 0; i < innerW; i++ {
		if lobsterBot == i && i+1 < innerW {
			sb.WriteString(cReset + "🦞" + cBorder)
			i++ // 🦞 is 2 columns wide — skip next position
		} else {
			sb.WriteRune('─')
		}
	}
	sb.WriteString("┘" + cReset + "\033[K")
	return sb.String()
}

// ── Inbox rendering ────────────────────────────────────────────────

func renderInboxLines(s *chatTUI, w, maxLines int) []string {
	lines := make([]string, 0, maxLines)

	if len(s.inbox) == 0 {
		lines = append(lines, padRight(cDim+"  No conversations"+cReset, w))
		lines = append(lines, padRight(cDim+"  F1 to start one"+cReset, w))
		return lines
	}

	if s.selectedIdx >= s.inboxScroll+maxLines {
		s.inboxScroll = s.selectedIdx - maxLines + 1
	}
	if s.selectedIdx < s.inboxScroll {
		s.inboxScroll = s.selectedIdx
	}

	end := s.inboxScroll + maxLines
	if end > len(s.inbox) {
		end = len(s.inbox)
	}

	for i := s.inboxScroll; i < end; i++ {
		e := s.inbox[i]
		short := e.PeerID
		if len(short) > 14 {
			short = short[:14]
		}

		prefix := "  "
		if i == s.selectedIdx {
			prefix = cTitle + " >" + cReset
		}

		arrow := cPeerInfo + "→" + cReset
		if e.Direction == "received" {
			arrow = cCoast + "←" + cReset
		}
		unread := ""
		if !e.Read && e.Direction == "received" {
			unread = " \033[92m●\033[0m"
		}

		ts := formatTimeLocal(e.CreatedAt)

		lineContent := fmt.Sprintf("%s%s %s %s%s", prefix, short, arrow, cDim+ts+cReset, unread)
		lines = append(lines, padRightAnsi(lineContent, w))
	}

	return lines
}

// ── Thread rendering (Reddit-style) ────────────────────────────────

func renderThreadLines(s *chatTUI, w, maxLines int) []string {
	lines := make([]string, 0, maxLines)

	if s.selectedPeerID() == "" {
		lines = append(lines, padRight(cDim+"  Select a conversation"+cReset, w))
		return lines
	}

	if len(s.thread) == 0 {
		lines = append(lines, padRight(cDim+"  No messages yet"+cReset, w))
		lines = append(lines, padRight(cDim+"  Tab to input area"+cReset, w))
		return lines
	}

	// Build rendered lines — Reddit style: header line + body line(s) per message
	type renderedLine struct {
		text string
	}
	var allLines []renderedLine

	// Thread from API is newest-first; reverse to show oldest at top
	reversed := make([]threadMsg, len(s.thread))
	for i, m := range s.thread {
		reversed[len(s.thread)-1-i] = m
	}

	pid := s.selectedPeerID()
	rep := s.peerRep[pid]

	for _, m := range reversed {
		ts := formatTimeLocal(m.CreatedAt)

		// Reddit-style: author line with emoji + rank + timestamp
		var authorLine string
		if m.Direction == "sent" {
			authorLine = fmt.Sprintf(" %s%s you%s  %s%s%s",
				cPeerInfo, "🦞", cReset,
				cDim, ts, cReset)
		} else {
			emoji := "🦐"
			rankColor := "\033[38;2;100;100;100m"
			rankName := "Peer"
			if rep != nil {
				emoji, rankColor = lobsterLevel(rep.Score)
				rankName = lobsterRankName(rep.Score)
			}
			shortPeer := pid
			if len(shortPeer) > 8 {
				shortPeer = shortPeer[:8]
			}
			authorLine = fmt.Sprintf(" %s%s %s%s %s  %s%s%s",
				rankColor, emoji, rankName, cReset,
				cCoast+shortPeer+cReset,
				cDim, ts, cReset)
		}
		allLines = append(allLines, renderedLine{text: padRightAnsi(authorLine, w)})

		// Message body with word-wrapping
		bodyW := w - 4 // indent
		if bodyW < 10 {
			bodyW = 10
		}
		bodyRunes := []rune(m.Body)
		for len(bodyRunes) > 0 {
			chunk := bodyRunes
			if len(chunk) > bodyW {
				chunk = chunk[:bodyW]
			}
			bodyLine := "   " + string(chunk)
			allLines = append(allLines, renderedLine{text: padRightAnsi(bodyLine, w)})
			bodyRunes = bodyRunes[len(chunk):]
		}

		// Separator
		sep := " " + cDim + strings.Repeat("·", w-2) + cReset
		allLines = append(allLines, renderedLine{text: padRightAnsi(sep, w)})
	}

	// Apply scroll: threadScroll 0 = bottom (newest visible)
	totalLines := len(allLines)
	maxScroll := totalLines - maxLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if s.threadScroll > maxScroll {
		s.threadScroll = maxScroll
	}

	endIdx := totalLines - s.threadScroll
	if endIdx < 0 {
		endIdx = 0
	}
	startIdx := endIdx - maxLines
	if startIdx < 0 {
		startIdx = 0
	}

	visible := allLines[startIdx:endIdx]

	// Pad top if needed
	if len(visible) < maxLines {
		padding := make([]renderedLine, maxLines-len(visible))
		for i := range padding {
			padding[i] = renderedLine{text: strings.Repeat(" ", w)}
		}
		visible = append(padding, visible...)
	}

	for _, rl := range visible {
		lines = append(lines, rl.text)
	}

	return lines
}

// ── Input rendering (3 rows) ───────────────────────────────────────

func renderInputLines(s *chatTUI, w, h int) []string {
	lines := make([]string, h)
	for i := range lines {
		lines[i] = ""
	}

	if s.newMode {
		prompt := cCoast + " To: " + cReset
		lines[0] = prompt + string(s.newPeerBuf)
		lines[1] = cDim + " Enter peer ID, then Enter to confirm" + cReset
		return lines
	}

	active := s.panel == chatPanelInput
	prompt := " > "
	text := string(s.inputBuf)

	// Split into lines if text is long
	lineW := w - 3 // prompt width
	if lineW < 10 {
		lineW = 10
	}

	textRunes := []rune(text)
	row := 0
	for start := 0; start < len(textRunes) && row < h; start += lineW {
		end := start + lineW
		if end > len(textRunes) {
			end = len(textRunes)
		}
		chunk := string(textRunes[start:end])
		if row == 0 {
			if active {
				lines[row] = prompt + chunk
			} else {
				lines[row] = cDim + prompt + chunk + cReset
			}
		} else {
			pad := "   "
			if active {
				lines[row] = pad + chunk
			} else {
				lines[row] = cDim + pad + chunk + cReset
			}
		}
		row++
	}
	if row == 0 {
		if active {
			lines[0] = prompt
		} else {
			lines[0] = cDim + prompt + cReset
		}
	}

	return lines
}

// ── Helpers ──────────────────────────────────────────────────────

func padRight(s string, w int) string {
	vl := visibleLen(s)
	if vl >= w {
		return truncToWidth(s, w)
	}
	return s + strings.Repeat(" ", w-vl)
}

func padRightAnsi(s string, w int) string {
	vl := visibleLen(s)
	if vl >= w {
		return truncToWidth(s, w)
	}
	return s + strings.Repeat(" ", w-vl)
}

func formatTimeLocal(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", ts)
		if err != nil {
			return ts
		}
	}
	// Convert to local timezone
	t = t.Local()
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return t.Format("15:04")
	case diff < 7*24*time.Hour:
		return t.Format("Mon 15:04")
	default:
		return t.Format("01/02 15:04")
	}
}

// ── API fetchers ───────────────────────────────────────────────────

func fetchInbox(base string) []inboxEntry {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(base + "/api/dm/inbox")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var msgs []inboxEntry
	json.NewDecoder(resp.Body).Decode(&msgs)
	return msgs
}

func fetchThreadMsgs(base, peerID string) []threadMsg {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/dm/thread/%s?limit=100", base, peerID))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var msgs []threadMsg
	json.NewDecoder(resp.Body).Decode(&msgs)
	return msgs
}

func sendChatMsg(base, peerID, body string) error {
	payload := fmt.Sprintf(`{"peer_id":%q,"body":%q}`, peerID, body)
	resp, err := http.Post(base+"/api/dm/send", "application/json", strings.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s", string(b))
	}
	return nil
}

func deleteInbox(base, peerID string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/dm/thread/%s", base, peerID), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
