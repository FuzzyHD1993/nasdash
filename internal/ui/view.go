package ui

import (
	"fmt"
	"strings"
	"time"

	"nasdash/internal/api"
)

func fit(s string, w int) string {
	r := []rune(s)
	if w <= 0 {
		return ""
	}
	if len(r) > w {
		if w <= 1 {
			return string(r[:w])
		}
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

func clip(s string, w int) string {
	r := []rune(s)
	if w <= 0 || len(r) <= w {
		return s
	}
	if w == 1 {
		return string(r[:1])
	}
	return string(r[:w-1]) + "…"
}

func bar(p float64, w int) string {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	n := int(float64(w) * p / 100)
	return strings.Repeat("█", n) + strings.Repeat("░", w-n)
}

func dot(up bool) string {
	if up {
		return okStyle.Render("●")
	}
	return errStyle.Render("○")
}

func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 120
	}
	h := m.height
	if h <= 0 {
		h = 40
	}

	head := titleStyle.Render("NAS Dashboard") + strings.Repeat(" ", max(1, w-14-len("NAS Dashboard"))) +
		dimStyle.Render(m.now.Format("15:04:05")) + " " + dot(m.up[m.active])

	tabs := make([]string, 0, tabCount)
	for i := 0; i < int(tabCount); i++ {
		label := fmt.Sprintf("%d %s", i+1, tabNames[i])
		if tab(i) == m.active {
			tabs = append(tabs, activeTabSty.Render(label))
		} else {
			tabs = append(tabs, inactiveTabSt.Render(label))
		}
	}
	tabline := strings.Join(tabs, " ")

	sep := dimStyle.Render(strings.Repeat("─", w))

	var body string
	switch m.active {
	case tabTorrents:
		body = m.viewTorrents(w, h-5)
	case tabSwarm:
		body = m.viewSwarm(w, h-5)
	case tabTasks:
		body = m.viewTasks(w, h-5)
	case tabMusic:
		body = m.viewMusic(w, h-5)
	case tabSystem:
		body = m.viewSystem(w, h-5)
	}

	foot := m.helpLine(w)
	if time.Since(m.toastAt) < 6*time.Second && m.toast != "" {
		if m.toastOK {
			foot = okStyle.Render("✓ " + clip(m.toast, w-3))
		} else {
			foot = errStyle.Render("✗ " + clip(m.toast, w-3))
		}
	}
	if m.confirmDel {
		foot = errStyle.Render("удалить торрент? y — да, n — нет")
	}

	return head + "\n" + tabline + "\n" + sep + "\n" + body + "\n" + sep + "\n" + fit(foot, w)
}

func (m Model) helpLine(w int) string {
	var s string
	switch m.active {
	case tabTorrents:
		s = "↑/↓ выбор · s поиск · a добавить · d удалить · p пауза/продолжить · r обновить · 1-5 вкладки · q выход"
	case tabSwarm:
		if m.swarmFocus {
			s = "введи задачу · ctrl+enter запустить · esc снять фокус · ctrl+c выход"
		} else {
			s = "enter — вернуться в поле ввода · ctrl+n/ctrl+p прокрутка ответа · 1-5 вкладки · q выход"
		}
	case tabTasks:
		s = "↑/↓ выбор · enter запустить · r обновить · 1-5 вкладки · q выход"
	case tabMusic:
		s = "space play/pause · n next · p prev · +/- громкость · r обновить · 1-5 вкладки · q выход"
	case tabSystem:
		s = "r обновить · 1-5 вкладки · q выход · автообновление каждые 2 с"
	}
	return helpStyle.Render(s)
}

// --- torrents ---

func (m Model) viewTorrents(w, h int) string {
	if m.searchMode {
		return m.viewSearch(w, h)
	}
	var b strings.Builder
	if m.addMode {
		b.WriteString("Добавить торрент (magnet):\n" + m.addIn.View() + "\n\n")
	}
	if m.tErr != nil {
		b.WriteString(errStyle.Render("ошибка: "+m.tErr.Error()) + "\n")
	}
	if m.tLoading && len(m.torrents) == 0 {
		b.WriteString(m.sp.View() + " загрузка…\n")
	}
	hdr := fit("  НАЗВАНИЕ", w-46) + fit("ПРОГРЕСС", 18) + fit("СТАТУС", 12) + fit("↓", 10) + "РАЗМЕР"
	b.WriteString(dimStyle.Render(clip(hdr, w)) + "\n")
	visible := max(1, h-2)
	start := 0
	if m.tCursor >= visible {
		start = m.tCursor - visible + 1
	}
	for i := start; i < len(m.torrents) && i < start+visible; i++ {
		t := m.torrents[i]
		line := "  " + fit(clip(t.Name(), w-48), w-48) +
			fit(fmt.Sprintf("%s %3.0f%%", bar(t.Progress, 10), t.Progress), 18) +
			fit(clip(t.Status, 11), 12) +
			fit(api.Speed(t.SpeedDownload), 10) +
			api.Bytes(t.Size)
		if i == m.tCursor {
			b.WriteString(selStyle.Render(clip("› "+line[0:], w)) + "\n")
		} else {
			b.WriteString(clip(line, w) + "\n")
		}
	}
	if len(m.torrents) == 0 && m.tErr == nil && !m.tLoading {
		b.WriteString(dimStyle.Render("пусто") + "\n")
	}
	return b.String()
}

func (m Model) viewSearch(w, h int) string {
	var b strings.Builder
	b.WriteString("Поиск (Enter — искать, esc — назад):\n" + m.searchIn.View() + "\n\n")
	if m.searchBusy {
		b.WriteString(m.sp.View() + " ищу…\n")
	}
	if m.searchErr != nil {
		b.WriteString(errStyle.Render(m.searchErr.Error()) + "\n")
	}
	for i, hit := range m.hits {
		if i >= max(1, h-5) {
			break
		}
		line := fmt.Sprintf("%s  %s  %s", fit(clip(hit.Title, w-26), w-26), fit(hit.Size, 10), hit.Seeders)
		if i == m.hCursor {
			b.WriteString(selStyle.Render(clip(line, w)) + "\n")
		} else {
			b.WriteString(clip(line, w) + "\n")
		}
	}
	if len(m.hits) == 0 && !m.searchBusy && m.searchErr == nil {
		b.WriteString(dimStyle.Render("нет результатов") + "\n")
	}
	return b.String()
}

// --- swarm ---

func (m Model) viewSwarm(w, h int) string {
	var b strings.Builder
	b.WriteString("Задача: " + m.swarmIn.View() + "\n\n")
	state := func(s int) string {
		switch s {
		case 1:
			return m.sp.View() + " работает"
		case 2:
			return okStyle.Render("готово")
		case 3:
			return errStyle.Render("ошибка")
		}
		return dimStyle.Render("ожидает")
	}
	panelH := 4
	for i, a := range swarmAgents() {
		txt := m.panelText[i]
		if txt == "" && m.panelState[i] == 0 {
			txt = ""
		}
		b.WriteString(boxStyle.Render(
			titleStyle.Render(a)+"  "+state(m.panelState[i])+"\n"+clipLines(txt, w-6, panelH),
		) + "\n")
	}
	b.WriteString(boxStyle.Render(titleStyle.Render("Архитектор") + "  " + state(m.archState) + "\n" +
		clipLines(scrolled(m.archText, m.archScroll), w-6, max(3, h-5*panelH-8))))
	return b.String()
}

func swarmAgents() []string { return []string{"DeepSeek", "GLM", "Bunny"} }

func clipLines(s string, w, maxLines int) string {
	if s == "" {
		return dimStyle.Render("—")
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	var out []string
	for _, ln := range lines {
		for len([]rune(ln)) > w && w > 0 {
			out = append(out, string([]rune(ln)[:w]))
			ln = string([]rune(ln)[w:])
		}
		out = append(out, ln)
		if len(out) >= maxLines {
			break
		}
	}
	if len(out) > maxLines {
		out = out[:maxLines]
	}
	return strings.Join(out, "\n")
}

func scrolled(s string, skip int) string {
	if skip <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if skip >= len(lines) {
		return ""
	}
	return strings.Join(lines[skip:], "\n")
}

// --- tasks ---

func (m Model) viewTasks(w, h int) string {
	var b strings.Builder
	if m.tasksErr != nil {
		b.WriteString(errStyle.Render("ошибка: "+m.tasksErr.Error()) + "\n")
	}
	hdr := fit("  ЗАДАЧА", w-32) + fit("РАСПИСАНИЕ", 16) + "СТАТУС"
	b.WriteString(dimStyle.Render(clip(hdr, w)) + "\n")
	visible := max(1, h-2)
	start := 0
	if m.kCursor >= visible {
		start = m.kCursor - visible + 1
	}
	for i := start; i < len(m.tasks) && i < start+visible; i++ {
		t := m.tasks[i]
		st := t.Status
		if st == "" {
			st = "scheduled"
		}
		if t.LastRun != "" {
			st += " " + t.LastRun
		}
		line := "  " + fit(clip(t.Name, w-34), w-34) + fit(t.Schedule, 16) + st
		if i == m.kCursor {
			b.WriteString(selStyle.Render(clip(line, w)) + "\n")
		} else {
			b.WriteString(clip(line, w) + "\n")
		}
	}
	if m.taskBusy != "" {
		b.WriteString("\n" + m.sp.View() + " запускаю " + m.taskBusy + "…\n")
	}
	return b.String()
}

// --- music ---

func (m Model) viewMusic(w, h int) string {
	var b strings.Builder
	if m.musicErr != nil {
		b.WriteString(errStyle.Render("music-service недоступен: "+m.musicErr.Error()) + "\n")
		b.WriteString(dimStyle.Render("сервис не публикует порт 8092 наружу; доступ идёт через gateway :8080/music") + "\n")
		return b.String()
	}
	st := m.music
	if st.Err != "" {
		b.WriteString(errStyle.Render(st.Err) + "\n")
	}
	title := st.Title
	if title == "" {
		title = "—"
	}
	b.WriteString(titleStyle.Render(clip(title, w)) + "\n")
	b.WriteString(dimStyle.Render(clip(st.Artist+" — "+st.Album, w)) + "\n\n")
	b.WriteString(fmt.Sprintf("Состояние: %s\n", st.State))
	b.WriteString(fmt.Sprintf("Громкость: %3d%% %s\n", st.Volume, bar(float64(st.Volume), 20)))
	if st.Duration > 0 {
		b.WriteString(fmt.Sprintf("%s / %s\n", mmss(st.Elapsed), mmss(st.Duration)))
	}
	return b.String()
}

func mmss(f float64) string {
	d := time.Duration(f * float64(time.Second))
	return fmt.Sprintf("%02d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// --- system ---

func (m Model) viewSystem(w, h int) string {
	var b strings.Builder
	if m.statsErr != nil {
		b.WriteString(errStyle.Render("ошибка: "+m.statsErr.Error()) + "\n")
	}
	s := m.stats
	b.WriteString(fmt.Sprintf("CPU: %-5.1f%% %s\n", s.CPU.Usage, bar(s.CPU.Usage, 30)))
	b.WriteString(clip(dimStyle.Render(s.CPU.Model), w) + "\n\n")
	ramPct := 0.0
	fmt.Sscanf(s.RAM.Percent, "%f", &ramPct)
	b.WriteString(fmt.Sprintf("RAM: %-5s %s  (%s / %s GB)\n\n", s.RAM.Percent, bar(ramPct, 30), s.RAM.UsedGB, s.RAM.TotalGB))
	b.WriteString(dimStyle.Render("ДИСКИ") + "\n")
	for i, d := range s.Disks {
		if i >= max(1, h-8) {
			break
		}
		p := 0.0
		fmt.Sscanf(d.Percent, "%f", &p)
		b.WriteString(fmt.Sprintf("%-14s %s %5.0f%%  %s / %s  %s\n",
			clip(d.Mount, 14), bar(p, 20), p, d.Used, d.Total, dimStyle.Render(d.Fstype)))
	}
	if len(s.GPU) > 0 {
		if e, ok := s.GPU["error"].(string); ok && e != "" {
			b.WriteString(dimStyle.Render("GPU: "+e) + "\n")
		}
	}
	return b.String()
}
