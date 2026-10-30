package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"nasdash/internal/api"
	"nasdash/internal/swarm"
)

type tab int

const (
	tabTorrents tab = iota
	tabSwarm
	tabTasks
	tabMusic
	tabSystem
	tabCount
)

var tabNames = []string{"Торренты", "Рой", "Задачи", "Музыка", "Система"}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	activeTabSty  = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("62")).Foreground(lipgloss.Color("231")).Padding(0, 1)
	inactiveTabSt = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selStyle      = lipgloss.NewStyle().Background(lipgloss.Color("236")).Bold(true)
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

type tickMsg time.Time

type torrentsMsg struct {
	list []api.Torrent
	err  error
}
type statsMsg struct {
	s   api.Stats
	err error
}
type tasksMsg struct {
	list []api.Task
	err  error
}
type musicMsg struct {
	s   api.MusicStatus
	err error
}
type searchMsg struct {
	hits []api.SearchHit
	err  error
}
type actionMsg struct {
	what string
	err  error
}
type swarmMsg struct {
	idx  int
	text string
	err  error
}
type archMsg struct {
	text string
	err  error
}

// Model is the whole application state.
type Model struct {
	api *api.Client

	active tab
	width  int
	height int
	now    time.Time

	up [5]bool // last request succeeded per tab

	sp spinner.Model

	// torrents
	torrents   []api.Torrent
	tCursor    int
	tErr       error
	tLoading   bool
	searchMode bool
	searchIn   textinput.Model
	hits       []api.SearchHit
	hCursor    int
	searchErr  error
	searchBusy bool
	addMode    bool
	addIn      textinput.Model
	confirmDel bool

	// swarm
	swarmIn    textinput.Model
	swarmFocus bool
	panelText  [3]string
	panelState [3]int // 0 idle, 1 running, 2 done, 3 error
	archText   string
	archState  int // 0 idle, 1 running, 2 done, 3 error
	swarmBusy  bool
	archScroll int
	question   string

	// tasks
	tasks      []api.Task
	kCursor    int
	tasksErr   error
	taskBusy   string
	taskResult string

	// music
	music    api.MusicStatus
	musicErr error

	// system
	stats    api.Stats
	statsErr error

	toast   string
	toastOK bool
	toastAt time.Time
}

func New(c *api.Client) Model {
	si := textinput.New()
	si.Placeholder = "поиск торрентов…"
	si.CharLimit = 120

	ai := textinput.New()
	ai.Placeholder = "magnet:?xt=urn:btih:…"
	ai.CharLimit = 1024

	wi := textinput.New()
	wi.Placeholder = "задача для роя…"
	wi.CharLimit = 1000
	wi.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		api:        c,
		active:     tabTorrents,
		now:        time.Now(),
		sp:         sp,
		searchIn:   si,
		addIn:      ai,
		swarmIn:    wi,
		taskBusy:   "",
		toastAt:    time.Now().Add(-time.Hour),
		swarmFocus: true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tick(), m.sp.Tick, m.fetchTorrents(), m.fetchTasks(), m.fetchMusic(), m.fetchStats())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) withCtx(fn func(ctx context.Context) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		return fn(ctx)
	}
}

func (m Model) fetchTorrents() tea.Cmd {
	m.tLoading = true
	return m.withCtx(func(ctx context.Context) tea.Msg {
		l, err := m.api.Torrents(ctx)
		return torrentsMsg{l, err}
	})
}

func (m Model) fetchTasks() tea.Cmd {
	return m.withCtx(func(ctx context.Context) tea.Msg {
		l, err := m.api.Tasks(ctx)
		return tasksMsg{l, err}
	})
}

func (m Model) fetchMusic() tea.Cmd {
	return m.withCtx(func(ctx context.Context) tea.Msg {
		s, err := m.api.MusicStatus(ctx)
		return musicMsg{s, err}
	})
}

func (m Model) fetchStats() tea.Cmd {
	return m.withCtx(func(ctx context.Context) tea.Msg {
		s, err := m.api.Stats(ctx)
		return statsMsg{s, err}
	})
}

func (m Model) toastf(ok bool, format string, a ...any) Model {
	m.toast = fmt.Sprintf(format, a...)
	m.toastOK = ok
	m.toastAt = time.Now()
	return m
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		cmds := []tea.Cmd{tick()}
		if m.active == tabSystem && m.now.Unix()%2 == 0 {
			cmds = append(cmds, m.fetchStats())
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd

	case torrentsMsg:
		m.tLoading = false
		m.tErr = msg.err
		m.up[tabTorrents] = msg.err == nil
		if msg.err == nil {
			m.torrents = msg.list
			if m.tCursor >= len(m.torrents) {
				m.tCursor = max(0, len(m.torrents)-1)
			}
		}
		return m, nil

	case statsMsg:
		m.statsErr = msg.err
		m.up[tabSystem] = msg.err == nil
		if msg.err == nil {
			m.stats = msg.s
		}
		return m, nil

	case tasksMsg:
		m.tasksErr = msg.err
		m.up[tabTasks] = msg.err == nil
		if msg.err == nil {
			m.tasks = msg.list
			if m.kCursor >= len(m.tasks) {
				m.kCursor = max(0, len(m.tasks)-1)
			}
		}
		return m, nil

	case musicMsg:
		m.musicErr = msg.err
		m.up[tabMusic] = msg.err == nil
		m.music = msg.s
		return m, nil

	case searchMsg:
		m.searchBusy = false
		m.searchErr = msg.err
		m.hits = msg.hits
		m.hCursor = 0
		return m, nil

	case actionMsg:
		m.taskBusy = ""
		m.confirmDel = false
		ok := msg.err == nil
		if ok {
			m = m.toastf(true, "%s: готово", msg.what)
		} else {
			m = m.toastf(false, "%s: %v", msg.what, msg.err)
		}
		return m, tea.Batch(m.fetchTorrents(), m.fetchTasks())

	case swarmMsg:
		m.panelState[msg.idx] = 2
		if msg.err != nil {
			m.panelState[msg.idx] = 3
			m.panelText[msg.idx] = msg.err.Error()
		} else {
			m.panelText[msg.idx] = msg.text
		}
		if m.allPanelsDone() && m.archState == 0 {
			m.archState = 1
			q := m.question
			answers := make([]string, len(m.panelText))
			copy(answers, m.panelText[:])
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
				defer cancel()
				txt, err := swarm.Architect(ctx, q, answers)
				return archMsg{txt, err}
			}
		}
		return m, nil

	case archMsg:
		m.swarmBusy = false
		if msg.err != nil {
			m.archState = 3
			m.archText = msg.err.Error()
		} else {
			m.archState = 2
			m.archText = msg.text
		}
		m.archScroll = 0
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Modal text inputs.
	if m.searchMode {
		if len(m.hits) > 0 {
			switch key {
			case "up", "k":
				if m.hCursor > 0 {
					m.hCursor--
				}
				return m, nil
			case "down", "j":
				if m.hCursor < len(m.hits)-1 {
					m.hCursor++
				}
				return m, nil
			case "enter":
				link := m.hits[m.hCursor].Link
				if link == "" {
					m = m.toastf(false, "у результата нет magnet/ссылки")
					return m, nil
				}
				m.searchMode = false
				m.searchIn.Blur()
				m.addMode = true
				m.addIn.SetValue(link)
				m.addIn.Focus()
				return m, nil
			}
		}
		switch key {
		case "esc":
			m.searchMode = false
			m.searchIn.Blur()
			return m, nil
		case "enter":
			q := strings.TrimSpace(m.searchIn.Value())
			if q == "" {
				return m, nil
			}
			m.searchBusy = true
			m.searchErr = nil
			api := m.api
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
				defer cancel()
				hits, err := api.Search(ctx, q)
				return searchMsg{hits, err}
			}
		}
		var cmd tea.Cmd
		m.searchIn, cmd = m.searchIn.Update(msg)
		return m, cmd
	}

	if m.addMode {
		switch key {
		case "esc":
			m.addMode = false
			m.addIn.Blur()
			return m, nil
		case "enter":
			v := strings.TrimSpace(m.addIn.Value())
			if v == "" {
				return m, nil
			}
			m.addMode = false
			m.addIn.Blur()
			api := m.api
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				return actionMsg{"добавление торрента", api.AddTorrent(ctx, v, "")}
			}
		}
		var cmd tea.Cmd
		m.addIn, cmd = m.addIn.Update(msg)
		return m, cmd
	}

	// Swarm prompt: typing goes into the input while focused.
	if m.active == tabSwarm && m.swarmFocus {
		switch key {
		case "esc":
			m.swarmFocus = false
			m.swarmIn.Blur()
			return m, nil
		case "ctrl+enter":
			return m.startSwarm()
		}
		var cmd tea.Cmd
		m.swarmIn, cmd = m.swarmIn.Update(msg)
		return m, cmd
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "1", "2", "3", "4", "5":
		m.active = tab(int(key[0] - '1'))
		return m, m.onTabEnter()
	case "tab":
		m.active = (m.active + 1) % tabCount
		return m, m.onTabEnter()
	case "shift+tab":
		m.active = (m.active + tabCount - 1) % tabCount
		return m, m.onTabEnter()
	}

	switch m.active {
	case tabTorrents:
		return m.keyTorrents(key)
	case tabSwarm:
		switch key {
		case "enter":
			m.swarmFocus = true
			m.swarmIn.Focus()
		case "ctrl+n":
			m.archScroll++
		case "ctrl+p":
			if m.archScroll > 0 {
				m.archScroll--
			}
		}
		return m, nil
	case tabTasks:
		return m.keyTasks(key)
	case tabMusic:
		return m.keyMusic(key)
	case tabSystem:
		if key == "r" {
			return m, m.fetchStats()
		}
	}
	return m, nil
}

func (m Model) onTabEnter() tea.Cmd {
	switch m.active {
	case tabTorrents:
		return m.fetchTorrents()
	case tabTasks:
		return m.fetchTasks()
	case tabMusic:
		return m.fetchMusic()
	case tabSystem:
		return m.fetchStats()
	}
	return nil
}

func (m Model) keyTorrents(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.tCursor > 0 {
			m.tCursor--
		}
	case "down", "j":
		if m.tCursor < len(m.torrents)-1 {
			m.tCursor++
		}
	case "r":
		return m, m.fetchTorrents()
	case "s":
		m.searchMode = true
		m.hits = nil
		m.searchErr = nil
		m.searchIn.Focus()
		return m, nil
	case "a":
		m.addMode = true
		m.addIn.Focus()
		return m, nil
	case "d":
		if len(m.torrents) > 0 {
			m.confirmDel = true
		}
	case "y":
		if m.confirmDel && len(m.torrents) > 0 {
			id := m.torrents[m.tCursor].ID
			api := m.api
			m.confirmDel = false
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				return actionMsg{"удаление " + id, api.DeleteTorrent(ctx, id)}
			}
		}
	case "n":
		m.confirmDel = false
	case "p":
		if len(m.torrents) > 0 {
			t := m.torrents[m.tCursor]
			action := "pause"
			if strings.Contains(strings.ToLower(t.Status), "paus") {
				action = "resume"
			}
			api := m.api
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				return actionMsg{action + " " + t.ID, api.TorrentAction(ctx, action, t.ID)}
			}
		}
	}
	return m, nil
}

func (m Model) keyTasks(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.kCursor > 0 {
			m.kCursor--
		}
	case "down", "j":
		if m.kCursor < len(m.tasks)-1 {
			m.kCursor++
		}
	case "r":
		return m, m.fetchTasks()
	case "enter":
		if len(m.tasks) == 0 {
			return m, nil
		}
		name := m.tasks[m.kCursor].Name
		m.taskBusy = name
		api := m.api
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			out, err := api.RunTask(ctx, name)
			if err != nil {
				return actionMsg{"запуск " + name, err}
			}
			return actionMsg{"запуск " + name + " → " + out, nil}
		}
	}
	return m, nil
}

func (m Model) keyMusic(key string) (tea.Model, tea.Cmd) {
	mk := func(action string) tea.Cmd {
		api := m.api
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return actionMsg{action, api.MusicAction(ctx, action)}
		}
	}
	switch key {
	case " ":
		st := strings.ToLower(m.music.State)
		if st == "play" {
			return m, mk("pause")
		}
		return m, mk("play")
	case "n":
		return m, mk("next")
	case "p":
		return m, mk("prev")
	case "+", "=":
		return m, m.setVolume(m.music.Volume + 5)
	case "-":
		return m, m.setVolume(m.music.Volume - 5)
	case "r":
		return m, m.fetchMusic()
	}
	return m, nil
}

func (m Model) setVolume(level int) tea.Cmd {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	api := m.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := api.MusicVolume(ctx, level)
		return actionMsg{fmt.Sprintf("громкость %d%%", level), err}
	}
}

func (m Model) startSwarm() (tea.Model, tea.Cmd) {
	q := strings.TrimSpace(m.swarmIn.Value())
	if q == "" || m.swarmBusy {
		return m, nil
	}
	m.question = q
	m.swarmBusy = true
	m.archState = 0
	m.archText = ""
	for i := range m.panelText {
		m.panelText[i] = ""
		m.panelState[i] = 1
	}
	cmds := make([]tea.Cmd, 0, len(swarm.Agents))
	for i, a := range swarm.Agents {
		i, a := i, a
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			txt, err := swarm.Run(ctx, a.Name, a.Model, q)
			return swarmMsg{i, txt, err}
		})
	}
	return m, tea.Batch(cmds...)
}

// allPanelsDone reports whether every swarm member has finished.
func (m Model) allPanelsDone() bool {
	for _, s := range m.panelState {
		if s != 2 && s != 3 {
			return false
		}
	}
	return true
}
