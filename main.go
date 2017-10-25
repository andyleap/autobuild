// autobuild project main.go
package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/andyleap/imterm"
	"github.com/andyleap/imterm/imtermbox"
	tb "github.com/nsf/termbox-go"
	"github.com/rjeczalik/notify"
)

var refresh chan struct{}

func Refresh() {
	select {
	case refresh <- struct{}{}:
	default:
	}
}

type OutTracker struct {
	buf *bytes.Buffer
}

func (ot *OutTracker) Write(b []byte) (n int, err error) {
	n, err = ot.buf.Write(b)

	RunOut.Store(string(ot.buf.Bytes()))

	Refresh()
	return
}

var RunOut atomic.Value

var Running *exec.Cmd

func Run() {
	if Running != nil && !Running.ProcessState.Exited() {
		Running.Process.Kill()
	}

	dir, _ := os.Getwd()
	cmdName := filepath.Base(dir)
	Running = exec.Command(cmdName)

	ot := &OutTracker{
		buf: &bytes.Buffer{},
	}

	Running.Stdout = ot
	Running.Stderr = ot

	Running.Run()
}

func Build() bool {
	cmd := exec.Command("go", "build", "-i")
	out, err := cmd.CombinedOutput()
	BuildOut.Store(BuildRet{
		time.Now(),
		string(out),
	})
	return err == nil
}

type BuildRet struct {
	t   time.Time
	out string
}

var BuildOut atomic.Value

func main() {
	RunOut.Store("")

	it, err := imterm.New(&imtermbox.TermAdapter{})
	if err != nil {
		log.Fatal(err)
	}
	refresh = make(chan struct{}, 2)

	tb.Init()
	defer tb.Close()
	tb.SetInputMode(tb.InputEsc | tb.InputMouse)
	go func() {
		for {
			e := tb.PollEvent()
			switch e.Type {
			case tb.EventMouse:
				button := imterm.MouseNone
				switch e.Key {
				case tb.MouseRelease:
					button = imterm.MouseRelease
				case tb.MouseLeft:
					button = imterm.MouseLeft
				case tb.MouseRight:
					button = imterm.MouseRight
				case tb.MouseMiddle:
					button = imterm.MouseMiddle
				case tb.MouseWheelUp:
					button = imterm.MouseWheelUp
				case tb.MouseWheelDown:
					button = imterm.MouseWheelDown
				}
				it.Mouse(e.MouseX, e.MouseY, button)
			case tb.EventKey:
				it.Keyboard(imterm.Key(e.Key), e.Ch)
			}
			Refresh()
		}
	}()
	Refresh()

	autoBuild := true
	autoRun := false

	watchChan := make(chan notify.EventInfo, 1)

	err = notify.Watch(".", watchChan, notify.Write)
	if err != nil {
		log.Fatal(err)
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	buildCountDown := int64(0)

	go func() {
		for range watchChan {
			if autoBuild {
				atomic.StoreInt64(&buildCountDown, 10)
			}
		}
	}()

	Build()

	go func() {
		for range ticker.C {
			if atomic.LoadInt64(&buildCountDown) > 0 {
				val := atomic.AddInt64(&buildCountDown, -1)
				if val == 0 {
					ret := Build()
					Refresh()
					if autoRun && ret {
						go Run()
					}
				}
			}
		}
	}()

	for range refresh {
		it.Start()

		autoBuild = it.Toggle(it.TermW/3, 3, "AutoBuild", autoBuild)
		it.SameLine()
		autoRun = it.Toggle(it.TermW/3, 3, "AutoRun", autoRun)
		it.SameLine()
		if it.Button(0, 3, "Quit") {
			break
		}
		br := BuildOut.Load().(BuildRet)

		it.Text(0, it.TermH/2, "Build: "+br.t.Format("15:04:05"), br.out)
		it.Text(0, 0, "Run", RunOut.Load().(string))
		it.Finish()
	}
}
