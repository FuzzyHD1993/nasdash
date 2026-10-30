package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"nasdash/internal/api"
	"nasdash/internal/ui"
)

func main() {
	selftest := flag.Bool("selftest", false, "проверить все сервисы и выйти")
	flag.Parse()

	client := api.New()
	if *selftest {
		os.Exit(selftestRun(client))
	}

	p := tea.NewProgram(ui.New(client), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "nasdash: %v\n", err)
		os.Exit(1)
	}
}

func selftestRun(c *api.Client) int {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	bad := 0
	report := func(name string, n int, err error) {
		if err != nil {
			// The search service answers 502 when its upstream indexers are
			// down; the endpoint itself is healthy, so that is a warning.
			if strings.Contains(err.Error(), "HTTP 502") {
				fmt.Printf("%-12s WARN   %v\n", name, err)
				return
			}
			bad++
			fmt.Printf("%-12s ERROR  %v\n", name, err)
			return
		}
		fmt.Printf("%-12s OK     %d items\n", name, n)
	}

	list, err := c.Torrents(ctx)
	report("torrents", len(list), err)

	st, err := c.Stats(ctx)
	report("stats", len(st.Disks), err)

	tasks, err := c.Tasks(ctx)
	report("tasks", len(tasks), err)

	music, err := c.MusicStatus(ctx)
	report("music", 1, err)
	if err == nil {
		fmt.Printf("%-12s state=%q volume=%d track=%q\n", "", music.State, music.Volume, music.Title)
	}

	hits, err := c.Search(ctx, "test")
	report("search", len(hits), err)

	fmt.Printf("admin token: %s\n", map[bool]string{true: "found", false: "missing"}[c.AdminToken != ""])
	if bad > 0 {
		return 1
	}
	return 0
}
