package webui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/manual"
	"github.com/borismilner/agentbox/internal/session"
)

// The assignment runner (M12/FR82). The daemon owns WHEN; this owns WHAT a run
// actually is, and the answer is deliberately unexciting: an ordinary session.
//
// That is the design decision worth keeping. A run could have been a private
// child with its output scraped, and it would have been less code - but then a
// run that went wrong would be a black box that mailed a summary. As a session
// it lands in the surface Boris already has: he can open it, read every tool
// call it made, and take it over.

// runPoll is how often the runner looks at the child's state. A run takes
// minutes; a quarter second is imperceptible on the finishing end and costs
// nothing on a laptop.
const runPoll = 250 * time.Millisecond

// runCeiling is the longest a single run may take before it is killed and
// recorded as overrun. It is not a knob: the number exists so a wedged child
// cannot hold its assignment's in-flight flag forever, which would mean the
// assignment silently never runs again. An hour is far past any honest run and
// far short of "never".
const runCeiling = time.Hour

// RunAssignment carries one assignment out and blocks until it is done. It is
// the daemon.Runner the scheduler calls.
func (u *UI) RunAssignment(req daemon.RunRequest) (summary, data string, err error) {
	cwd := strings.TrimSpace(req.Dir)
	if cwd == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cwd = home
		}
	}
	mode := req.Mode
	if mode == "" {
		// Full, not the panel's default. An assignment that cannot run a
		// command or write a file is a scheduled reading exercise; plan mode is
		// there for the assignment whose author deliberately wants one.
		mode = "full"
	}
	id, err := u.sess.spawn(spawnReq{
		cwd:   cwd,
		mode:  mode,
		model: req.Model,
		brief: manual.Assignment(),
		label: req.Name,
		// The human's selection is theirs. An assignment firing while the panel
		// is open must not move them off the conversation they are typing into.
		background: true,
	})
	if err != nil {
		return "", "", err
	}
	if req.OnSession != nil {
		req.OnSession(id)
	}
	u.log.Info("webui.assignment_started", "component", "webui", "assignment", req.AssignmentID,
		"run", req.RunID, "session", id, "trigger", req.Trigger)

	if err := u.sess.Send(id, req.Prompt); err != nil {
		return "", "", err
	}
	// Send moves the driver to working before it returns, so there is no window
	// here in which an unstarted run reads as a finished one.
	final, err := u.awaitRun(id)
	u.sess.finishRun(id)
	if err != nil {
		return "", "", err
	}
	summary, data = splitReport(final)
	if summary == "" && data == "" {
		// A child that exited without writing a word has not carried the
		// assignment out, whatever it did on the way. Recording that as a
		// successful run with an empty summary is how an assignment quietly
		// stops working for a month.
		return "", "", fmt.Errorf("the run ended without reporting anything; open its session to see what it did")
	}
	u.log.Info("webui.assignment_finished", "component", "webui", "assignment", req.AssignmentID,
		"run", req.RunID, "session", id, "data", data != "")
	return summary, data, nil
}

// awaitRun waits for the child to stop working and returns its last assistant
// message. An error state is the run's error: a child that crashed halfway
// through has not carried the assignment out, whatever it said before it did.
func (u *UI) awaitRun(id string) (string, error) {
	deadline := time.Now().Add(runCeiling)
	for {
		drv := u.sess.driver(id)
		if drv == nil {
			return "", fmt.Errorf("the run's session went away before it finished")
		}
		switch drv.State() {
		case session.StateWorking:
		case session.StateError:
			if msg := drv.LastError(); msg != "" {
				return "", fmt.Errorf("%s", msg)
			}
			return "", fmt.Errorf("the session failed")
		default:
			return lastAssistantText(drv.Turns()), nil
		}
		if time.Now().After(deadline) {
			u.sess.Close(id)
			return "", fmt.Errorf("the run passed %s without finishing and was stopped", runCeiling)
		}
		time.Sleep(runPoll)
	}
}

// driver hands out a session's driver by id, or nil.
func (s *sessions) driver(id string) *session.Driver {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ls := s.find(id); ls != nil {
		return ls.drv
	}
	return nil
}

// finishRun ends the child but keeps the conversation.
//
// Stop rather than Close: the row stays in the switcher and the transcript
// stays readable, which is the whole reason a run is a session. Stopping the
// child matters as much - a daily assignment that left one alive per run would
// have thirty `claude` processes idling by the end of the month.
func (s *sessions) finishRun(id string) {
	s.mu.Lock()
	ls := s.find(id)
	s.mu.Unlock()
	if ls == nil {
		return
	}
	s.save(ls) // reopenable tomorrow, with its context, not just its text
	ls.drv.Stop()
	s.touch()
}

// lastAssistantText is the run's report: the final assistant message, prose
// only. Tool calls and thinking are in the transcript for anybody who opens the
// session; a summary made of them would be a log, not a report.
func lastAssistantText(turns []session.Turn) string {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role != session.RoleAssistant {
			continue
		}
		var parts []string
		for _, seg := range turns[i].Segments {
			if seg.Kind == session.SegText && strings.TrimSpace(seg.Text) != "" {
				parts = append(parts, strings.TrimSpace(seg.Text))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	return ""
}

// dataFence is how a run hands back something to keep. The brief
// (manual.Assignment) tells the agent to end with one, and splitReport takes it
// out of the prose: what the human reads is a report, and what accumulates over
// a month is a series.
const dataFence = "```agentbox-data"

// splitReport separates the summary from the data block. A fence with no
// closing line is left in the prose rather than swallowing the rest of the
// report - a half-written block is a mistake worth seeing, not a reason to lose
// the summary.
func splitReport(final string) (summary, data string) {
	head, rest, ok := strings.Cut(final, dataFence)
	if !ok {
		return strings.TrimSpace(final), ""
	}
	_, body, ok := strings.Cut(rest, "\n")
	if !ok {
		return strings.TrimSpace(final), ""
	}
	block, tail, ok := strings.Cut(body, "```")
	if !ok {
		return strings.TrimSpace(final), ""
	}
	return strings.TrimSpace(head + tail), strings.TrimSpace(block)
}
