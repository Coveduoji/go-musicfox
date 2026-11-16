package ui

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anhoder/foxful-cli/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/playlist"
	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/utils/likelist"
)

type likeRoundTripper func(*http.Request) (*http.Response, error)

func (f likeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type likeOperationController struct {
	n    *Netease
	done chan struct{}
}

func (c *likeOperationController) KeyMsgHandle(msg tea.KeyMsg, a *model.App) (bool, model.Page, tea.Cmd) {
	if msg.String() != "x" {
		return false, nil, nil
	}
	likeSong(c.n, true, false)
	likeSong(c.n, false, false)
	close(c.done)
	return true, a.Main(), tea.Quit
}

func TestOriginalLikeOperationReturnsToEventLoop(t *testing.T) {
	// Isolate the live event loop so a regression cannot hang the entire test suite.
	if os.Getenv("MUSICFOX_LIKE_REGRESSION_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOriginalLikeOperationReturnsToEventLoop$", "-test.count=1")
		cmd.Env = append(os.Environ(), "MUSICFOX_LIKE_REGRESSION_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("like operation did not return to the event loop: %v\n%s", err, output)
		}
		return
	}

	previousConfig := configs.AppConfig
	configs.AppConfig = &configs.Config{}
	defer func() { configs.AppConfig = previousConfig }()
	const songID = 94117
	defer likelist.SetLikeSong(songID, false)
	previousTransport := http.DefaultTransport
	var calls atomic.Int32
	http.DefaultTransport = likeRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":200}`)), Request: r}, nil
	})
	defer func() { http.DefaultTransport = previousTransport }()
	n := &Netease{user: &structs.User{UserId: 1, MyLikePlaylistID: 2}}
	pm := playlist.NewPlaylistManager()
	if err := pm.Initialize(0, []structs.Song{{Id: songID, Name: "test"}}); err != nil {
		t.Fatal(err)
	}
	n.player = &Player{playlistManager: pm}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	controller := &likeOperationController{n: n, done: make(chan struct{})}
	opts := model.DefaultOptions()
	opts.EnableStartup = false
	opts.MainMenu = &playingTestMenu{}
	opts.MainMenuTitle = &model.MenuItem{Title: "test"}
	opts.KBControllers = []model.KeyboardController{controller}
	opts.InitHook = func(a *model.App) { _, _ = a.Update(tea.WindowSizeMsg{Width: 120, Height: 40}) }
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	opts.TeaOptions = []tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(io.Discard)}
	n.App = model.NewApp(opts)
	exited := make(chan struct{})
	go func() { _ = n.App.Run(); close(exited) }()
	// Let the framework finish its initial page tick before simulating a click.
	go func() { time.Sleep(100 * time.Millisecond); _, _ = writer.Write([]byte("x")); _ = writer.Close() }()
	select {
	case <-controller.done:
	case <-time.After(time.Second):
		cancel()
		<-exited
		t.Fatal("successful like operation blocked the UI event loop")
	}
	cancel()
	<-exited
	if calls.Load() != 2 || likelist.IsLikeSong(songID) {
		t.Fatal("existing like/unlike requests did not complete")
	}
}
