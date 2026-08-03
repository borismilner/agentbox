package hand

import (
	"fmt"
	"strconv"
	"strings"
)

// A script is the form this feature is actually used in. One call that moves,
// clicks, types and waits beats six calls that each pay for a process, an X
// connection and a round trip, and it keeps the interesting property: the whole
// sequence is parsed and checked before the first event is sent, so a typo in
// step nine cannot leave the desktop half-driven with a button held down.
//
//	window agentbox          # coordinates below are inside that window
//	move 25% -46          # 25% across, 46px up from the bottom edge
//	click
//	type Ship it
//	key ctrl+Return
//	wait 400
//
// Blank lines and # comments are ignored. `type` and `window` take the rest of
// the line verbatim, so no quoting rules to remember.

// Op is one kind of step.
type Op string

const (
	OpWindow Op = "window"
	OpScreen Op = "screen"
	OpMove   Op = "move"
	OpClick  Op = "click"
	OpDouble Op = "double"
	OpDrag   Op = "drag"
	OpScroll Op = "scroll"
	OpType   Op = "type"
	OpKey    Op = "key"
	OpWait   Op = "wait"
	OpSpeed  Op = "speed"
	OpWPM    Op = "wpm"
)

// Step is one parsed line.
type Step struct {
	Op   Op
	Line int
	Raw  string

	Text   string   // window title, or the text to type
	Keys   []string // key combinations, in order
	X, Y   Coord    // move / click / drag from
	To     bool     // this step moves before it acts
	X2, Y2 Coord    // drag to
	Button byte     // 1 left, 2 middle, 3 right, 4-7 wheel
	N      int      // scroll notches (+down), wait milliseconds
	F      float64  // speed multiplier
}

// Coord is one axis of a position, in the frame the script is currently in.
// The spellings exist because the useful positions are rarely absolute pixels:
// a card's buttons sit a fixed distance from its bottom edge whatever its height,
// and the middle of a window is the middle whatever its size.
//
//	400     400 pixels from the left/top edge
//	-46     46 pixels from the right/bottom edge
//	60%     60% of the way across/down
//	-25%    25% of the way in from the right/bottom edge
//	center  the middle of that axis
//	~       leave this axis where the pointer already is
//	~+30    30 pixels right/down of the pointer
type Coord struct {
	Kind  CoordKind
	Value float64
}

type CoordKind int

const (
	CoordEdge    CoordKind = iota // Value pixels from the near edge
	CoordFromEnd                  // Value pixels from the far edge
	CoordFrac                     // Value in 0..1 from the near edge
	CoordFracEnd                  // Value in 0..1 from the far edge
	CoordCenter
	CoordPointer // Value pixels from where the pointer is
)

// Resolve turns one axis into a root coordinate. origin and size describe the
// current frame on that axis; at is where the pointer is on it.
func (c Coord) Resolve(origin, size, at int) int {
	switch c.Kind {
	case CoordFromEnd:
		return origin + size - int(c.Value)
	case CoordFrac:
		return origin + int(c.Value*float64(size))
	case CoordFracEnd:
		return origin + size - int(c.Value*float64(size))
	case CoordCenter:
		return origin + size/2
	case CoordPointer:
		return at + int(c.Value)
	default:
		return origin + int(c.Value)
	}
}

// ParseCoord reads one coordinate token.
func ParseCoord(tok string) (Coord, error) {
	t := strings.TrimSpace(tok)
	switch {
	case t == "":
		return Coord{}, fmt.Errorf("empty coordinate")
	case strings.EqualFold(t, "center"), strings.EqualFold(t, "middle"):
		return Coord{Kind: CoordCenter}, nil
	case t == "~":
		return Coord{Kind: CoordPointer}, nil
	case strings.HasPrefix(t, "~"):
		v, err := strconv.ParseFloat(strings.TrimPrefix(t, "~"), 64)
		if err != nil {
			return Coord{}, fmt.Errorf("%q is not a pointer-relative offset", tok)
		}
		return Coord{Kind: CoordPointer, Value: v}, nil
	}

	pct := strings.HasSuffix(t, "%")
	t = strings.TrimSuffix(t, "%")
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return Coord{}, fmt.Errorf("%q is not a coordinate", tok)
	}
	fromEnd := v < 0 || t == "-0"
	if v < 0 {
		v = -v
	}
	switch {
	case pct && fromEnd:
		return Coord{Kind: CoordFracEnd, Value: v / 100}, nil
	case pct:
		return Coord{Kind: CoordFrac, Value: v / 100}, nil
	case fromEnd:
		return Coord{Kind: CoordFromEnd, Value: v}, nil
	default:
		return Coord{Kind: CoordEdge, Value: v}, nil
	}
}

// buttons a script may name.
var buttonNames = map[string]byte{
	"left": 1, "l": 1, "middle": 2, "m": 2, "right": 3, "r": 3,
}

func parseButton(tok string) (byte, error) {
	if b, ok := buttonNames[strings.ToLower(tok)]; ok {
		return b, nil
	}
	n, err := strconv.Atoi(tok)
	if err != nil || n < 1 || n > 9 {
		return 0, fmt.Errorf("%q is not a mouse button (left, middle, right, or 1-9)", tok)
	}
	return byte(n), nil
}

// ParseScript reads a whole script. Every step is checked here so that Run can
// assume it is valid, and so a bad line is reported with its number before
// anything moves.
func ParseScript(src string) ([]Step, error) {
	var out []Step
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		st, err := ParseStep(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		st.Line, st.Raw = i+1, line
		out = append(out, st)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the script has no steps")
	}
	return out, nil
}

// ParseStep reads one line.
func ParseStep(line string) (Step, error) {
	op, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	fields := strings.Fields(rest)
	st := Step{Op: Op(strings.ToLower(op))}

	switch st.Op {
	case OpWindow:
		if rest == "" {
			return st, fmt.Errorf("window needs a title to look for")
		}
		st.Text = rest

	case OpScreen:
		if rest != "" {
			return st, fmt.Errorf("screen takes no arguments")
		}

	case OpType:
		if rest == "" {
			return st, fmt.Errorf("type needs something to type")
		}
		st.Text = rest

	case OpKey:
		if len(fields) == 0 {
			return st, fmt.Errorf("key needs a combination, for example ctrl+alt+t or Escape")
		}
		st.Keys = fields

	case OpMove:
		if len(fields) != 2 {
			return st, fmt.Errorf("move needs an x and a y, for example: move 25%% -46")
		}
		var err error
		if st.X, err = ParseCoord(fields[0]); err != nil {
			return st, err
		}
		if st.Y, err = ParseCoord(fields[1]); err != nil {
			return st, err
		}
		st.To = true

	case OpClick, OpDouble:
		st.Button = 1
		var err error
		switch len(fields) {
		case 0:
		case 1:
			if st.Button, err = parseButton(fields[0]); err != nil {
				return st, err
			}
		case 2, 3:
			if st.X, err = ParseCoord(fields[0]); err != nil {
				return st, err
			}
			if st.Y, err = ParseCoord(fields[1]); err != nil {
				return st, err
			}
			st.To = true
			if len(fields) == 3 {
				if st.Button, err = parseButton(fields[2]); err != nil {
					return st, err
				}
			}
		default:
			return st, fmt.Errorf("%s takes an optional button, or x y, or x y button", st.Op)
		}

	case OpDrag:
		if len(fields) != 4 {
			return st, fmt.Errorf("drag needs x1 y1 x2 y2")
		}
		var err error
		for i, dst := range []*Coord{&st.X, &st.Y, &st.X2, &st.Y2} {
			if *dst, err = ParseCoord(fields[i]); err != nil {
				return st, err
			}
		}
		st.To, st.Button = true, 1

	case OpScroll:
		if len(fields) != 1 {
			return st, fmt.Errorf("scroll needs a number of notches (negative scrolls up)")
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil || n == 0 {
			return st, fmt.Errorf("%q is not a number of notches", fields[0])
		}
		st.N = n

	case OpWait:
		if len(fields) != 1 {
			return st, fmt.Errorf("wait needs milliseconds")
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil || n < 0 {
			return st, fmt.Errorf("%q is not a number of milliseconds", fields[0])
		}
		st.N = n

	case OpSpeed:
		if len(fields) != 1 {
			return st, fmt.Errorf("speed needs a multiplier, for example 1.5")
		}
		f, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || f <= 0 {
			return st, fmt.Errorf("%q is not a speed multiplier", fields[0])
		}
		st.F = f

	case OpWPM:
		if len(fields) != 1 {
			return st, fmt.Errorf("wpm needs a typing speed, for example 240")
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil || n <= 0 {
			return st, fmt.Errorf("%q is not a typing speed", fields[0])
		}
		st.N = n

	default:
		return st, fmt.Errorf("unknown step %q (window, screen, move, click, double, drag, scroll, type, key, wait, speed, wpm)", op)
	}
	return st, nil
}
