package hole

import (
	"math/bits"
	"math/rand/v2"
	"strings"
)

const (
	tetrisWidth  = 10
	tetrisHeight = 20

	// Games end once the stack grows taller than this, so that pieces can
	// always spawn and move freely at the top of the matrix.
	tetrisMaxStack = 12
)

type tetrisCell struct{ row, col int }

type tetrisPiece struct {
	col        int              // Column of the left edge of the bounding box.
	rotations  [4][4]tetrisCell // Cells of each orientation, clockwise.
	maxLines   [4]int           // Most lines each orientation can clear.
	placements []tetrisPlacement
}

// A rotation & column the piece can drop from, with the shortest keys.
type tetrisPlacement struct {
	rot, col int
	keys     string
}

var tetrisPieces = map[byte]*tetrisPiece{}

const tetrisPieceNames = "IJLOSTZ"

func init() {
	// Spawn orientations, as rows of their bounding box from top to bottom.
	for name, rows := range map[byte][]string{
		'I': {"....", "####", "....", "...."},
		'J': {"#..", "###", "..."},
		'L': {"..#", "###", "..."},
		'O': {"##", "##"},
		'S': {".##", "##.", "..."},
		'T': {".#.", "###", "..."},
		'Z': {"##.", ".##", "..."},
	} {
		size := len(rows)
		piece := &tetrisPiece{col: 5 - (size+1)/2}

		var cells []tetrisCell
		for row, line := range rows {
			for col, char := range line {
				if char == '#' {
					cells = append(cells, tetrisCell{row, col})
				}
			}
		}

		for rot := range 4 {
			for i, c := range cells {
				piece.rotations[rot][i] = c

				// Rotate clockwise within the bounding box for the next one.
				cells[i] = tetrisCell{c.col, size - 1 - c.row}
			}
		}

		// The O piece is the same in every rotation.
		rotations := 4
		if name == 'O' {
			rotations = 1
		}

		for rot := range rotations {
			var shape tetrisBoard
			for _, c := range piece.rotations[rot] {
				shape[c.row] |= 1 << c.col
			}
			piece.maxLines[rot] = len(shape.clearableRows())

			// Shifts blocked by a wall are skipped, they duplicate shorter ones.
			for col := -3; col < tetrisWidth; col++ {
				if (&tetrisBoard{}).fits(tetrisFalling{piece, rot, col, 0}) {
					piece.placements = append(piece.placements,
						tetrisPlacement{rot, col, piece.keys(rot, col)})
				}
			}
		}

		tetrisPieces[name] = piece
	}
}

// The shortest keys to place a piece with the given rotation & column.
func (p *tetrisPiece) keys(rot, col int) string {
	keys := strings.Repeat("C", rot)
	if col < p.col {
		keys += strings.Repeat("L", p.col-col)
	} else {
		keys += strings.Repeat("R", col-p.col)
	}
	return keys + "D"
}

// Each row is a bitmask, with a bit per column.
type tetrisBoard [tetrisHeight]uint16

const tetrisFullRow = 1<<tetrisWidth - 1

type tetrisFalling struct {
	piece         *tetrisPiece
	rot, col, row int
}

func (b *tetrisBoard) fits(f tetrisFalling) bool {
	for _, c := range f.piece.rotations[f.rot] {
		c.row += f.row
		c.col += f.col
		if c.col < 0 || c.col >= tetrisWidth || c.row >= tetrisHeight || b[c.row]&(1<<c.col) != 0 {
			return false
		}
	}
	return true
}

// Play the keys of a single piece, returning the number of cleared lines.
// Moves that would collide are ignored, keys after the hard drop are ignored.
func (b *tetrisBoard) play(name byte, keys string) int {
	piece := tetrisPieces[name]
	f := tetrisFalling{piece: piece, col: piece.col}

	for _, key := range keys {
		next := f
		switch key {
		case 'L':
			next.col--
		case 'R':
			next.col++
		case 'C':
			next.rot = (next.rot + 1) % 4
		case 'D':
			return b.lock(b.drop(next))
		}
		if b.fits(next) {
			f = next
		}
	}

	return -1
}

// Hard drop a piece as far down as it goes.
func (b *tetrisBoard) drop(f tetrisFalling) tetrisFalling {
	// Rows above the stack are empty, skip straight past them.
	f.row = max(f.row, tetrisHeight-b.stackHeight()-4)
	for f.row++; b.fits(f); f.row++ {
	}
	f.row--
	return f
}

func (b *tetrisBoard) lock(f tetrisFalling) int {
	for _, c := range f.piece.rotations[f.rot] {
		b[f.row+c.row] |= 1 << (f.col + c.col)
	}

	lines := 0
	for row := tetrisHeight - 1; row >= 0; {
		if b[row] == tetrisFullRow {
			copy(b[1:row+1], b[:row])
			b[0] = 0
			lines++
		} else {
			row--
		}
	}

	return lines
}

func (b *tetrisBoard) stackHeight() int {
	for row := range tetrisHeight {
		if b[row] != 0 {
			return tetrisHeight - row
		}
	}
	return 0
}

// The rows of a dropped piece that can be part of a full row. They can't be
// if a cell of the piece below has to fall through the rest of the row.
func (b *tetrisBoard) clearableRows() (rows []int) {
	var below uint16
	for row := tetrisHeight - 1; row >= 0; row-- {
		if b[row] != 0 && below&^b[row] == 0 {
			rows = append(rows, row)
		}
		below |= b[row]
	}
	return
}

// The best number of lines the next piece can clear, and the shortest keys.
func tetrisBest(board tetrisBoard, name byte) (best int, bestKeys string) {
	for _, p := range tetrisPieces[name].placements {
		b := board
		if lines := b.play(name, p.keys); lines > best || bestKeys == "" {
			best, bestKeys = lines, p.keys
		}
	}
	return
}

// Parse a game, returning the board before the last piece and its name.
func tetrisReplay(game string) (board tetrisBoard, next byte) {
	tokens := strings.Fields(game)
	for _, token := range tokens[:len(tokens)-1] {
		board.play(token[0], token[1:])
	}
	return board, tokens[len(tokens)-1][0]
}

// The cells the bot must keep empty, the ones it should fill, and whether
// clearing four lines at once is worth filling protected cells for.
type tetrisGoal struct {
	protected, targets tetrisBoard
	tetrises           bool
}

// Score a board for the bot that generates the histories, higher is better.
// It stacks cleanly while keeping the protected cells empty.
func tetrisScore(b *tetrisBoard, goal *tetrisGoal, lines int) int {
	var heights [tetrisWidth]int
	var seen uint16
	holes, blocked, filled, covered := 0, 0, 0, 0
	for row := range tetrisHeight {
		// Columns appearing in this row, and empty cells below the surface.
		for top := b[row] &^ seen; top != 0; top &= top - 1 {
			heights[bits.TrailingZeros16(top)] = tetrisHeight - row
		}
		seen |= b[row]
		holes += bits.OnesCount16(seen &^ b[row])
		covered += bits.OnesCount16(seen &^ b[row] & goal.targets[row])
		blocked += bits.OnesCount16(b[row] & goal.protected[row])
		filled += bits.OnesCount16(b[row] & goal.targets[row])
	}

	// Filling the rows the answer clears matters, burying their gaps ruins it.
	score := -40*holes - 1000*blocked + 60*filled - 1000*covered
	if lines == 4 && goal.tetrises {
		score += 1000
	}

	// Columns protected all the way up are wells, their height doesn't count.
	wells := goal.protected[0]
	for col, height := range heights {
		if wells&(1<<col) != 0 {
			continue
		}
		score -= height * height
		if col > 0 && wells&(1<<(col-1)) == 0 {
			score -= 3 * max(height-heights[col-1], heights[col-1]-height)
		}
	}
	return score
}

// Turn the shortest keys of a placement into something a human might press:
// rotations & moves interleaved, wasted moves, and moves bumping into walls.
func tetrisHumanize(keys string) string {
	moves := []byte(strings.TrimSuffix(keys, "D"))

	for range rand.IntN(3) {
		switch rand.IntN(4) {
		case 0:
			moves = append(moves, 'L', 'R')
		case 1:
			moves = append(moves, 'R', 'L')
		case 2:
			moves = append(moves, 'C', 'C', 'C', 'C')
		case 3:
			moves = append(moves, randChoice([]byte("LR")))
		}
	}

	// Interleaving can make rotations & moves bump into walls.
	if randBool() {
		shuffle(moves)
	}

	return string(moves) + "D"
}

// Play a piece & its keys the way the bot likes, returning them as a token.
func tetrisBotMove(board *tetrisBoard, goal *tetrisGoal) string {
	// Rank the placements of a few random pieces by score, picking the piece
	// as well as the placement keeps the stack clean.
	names := shuffle([]byte(tetrisPieceNames))[:randInt(1, 3)]

	type option struct {
		name  byte
		keys  string
		score int
	}
	// The bot's pieces never go below the top of the stack before dropping,
	// so they land on whichever column is the highest under them.
	var tops [tetrisWidth]int
	for col := range tetrisWidth {
		for tops[col] = 0; tops[col] < tetrisHeight && board[tops[col]]&(1<<col) == 0; tops[col]++ {
		}
	}

	var options []option
	for _, name := range names {
		piece := tetrisPieces[name]
		for _, p := range piece.placements {
			f := tetrisFalling{piece, p.rot, p.col, tetrisHeight}
			for _, c := range piece.rotations[p.rot] {
				f.row = min(f.row, tops[p.col+c.col]-1-c.row)
			}

			b := *board
			lines := b.lock(f)
			options = append(options, option{name, p.keys, tetrisScore(&b, goal, lines)})
		}
	}

	// Usually pick the best one, otherwise the second best.
	best, second := options[0], options[0]
	for _, o := range shuffle(options)[1:] {
		if o.score > best.score {
			best, second = o, best
		} else if o.score > second.score || second == best {
			second = o
		}
	}

	choice := best
	if rand.IntN(10) == 0 {
		choice = second
	}

	// Add noise to the keys, as long as the piece still lands in the same place.
	keys := choice.keys
	want := *board
	want.play(choice.name, keys)
	for range 5 {
		noisy := tetrisHumanize(keys)
		if got := *board; got.play(choice.name, noisy) >= 0 && got == want {
			keys = noisy
			break
		}
	}

	*board = want
	return string(choice.name) + keys
}

// Generate a game whose answer is a piece & rotation picked up front, among
// those able to clear the given number of lines.
func tetrisTest(lines int) test {
	type orientation struct {
		name byte
		rot  int
	}
	var orientations []orientation
	for _, name := range []byte(tetrisPieceNames) {
		for rot, most := range tetrisPieces[name].maxLines {
			if most >= lines {
				orientations = append(orientations, orientation{name, rot})
			}
		}
	}

	o := randChoice(orientations)
	for {
		if t, ok := tetrisTryTest(o.name, o.rot, lines); ok {
			return t
		}
	}
}

func tetrisTryTest(name byte, rot, lines int) (test, bool) {
	var board tetrisBoard
	var tokens []string

	// First play freely around a well, which may clear some lines.
	free := tetrisGoal{tetrises: true}
	well := uint16(1) << rand.IntN(tetrisWidth)
	for row := range free.protected {
		free.protected[row] = well
	}

	// Then plan where the answer lands on the stack, playing on until it can.
	piece := tetrisPieces[name]
	cols := make([]int, tetrisWidth+3)
	for i := range cols {
		cols[i] = i - 3
	}

	var keys string
	var goal tetrisGoal
	for start := randInt(0, 20); keys == ""; {
		if len(tokens) >= start {
			for _, col := range shuffle(cols) {
				var ok bool
				if goal, ok = tetrisPlan(&board, piece, rot, col, lines); ok {
					keys = piece.keys(rot, col)
					break
				}
			}
		}

		if keys == "" {
			tokens = append(tokens, tetrisBotMove(&board, &free))
			if len(tokens) > 30 || board.stackHeight() > tetrisMaxStack {
				return test{}, false
			}
		}
	}

	// Fill the stack around the answer, until it clears that many lines.
	length := randInt(4, 30)
	for end := len(tokens) + 20; len(tokens) < end && board.stackHeight() <= tetrisMaxStack; {
		if len(tokens) >= length {
			b := board
			if best, _ := tetrisBest(board, name); best == lines && b.play(name, keys) == lines {
				return test{strings.Join(append(tokens, string(name)), " "), keys}, true
			}
		}

		tokens = append(tokens, tetrisBotMove(&board, &goal))
	}

	return test{}, false
}

// Plan the answer to land in the given column. The cells to keep empty are the
// answer, everything above it, and a hole in each row that it mustn't clear.
// The rest of the rows it clears must be fillable from above.
func tetrisPlan(
	board *tetrisBoard, piece *tetrisPiece, rot, col, lines int,
) (goal tetrisGoal, ok bool) {
	f := tetrisFalling{piece, rot, col, 0}
	if !board.fits(f) {
		return
	}
	f = board.drop(f)

	var answer tetrisBoard
	for _, c := range piece.rotations[rot] {
		answer[f.row+c.row] |= 1 << (f.col + c.col)
	}

	clears := map[int]bool{}
	for _, row := range shuffle(answer.clearableRows())[:lines] {
		clears[row] = true
	}

	var answerAbove, boardAbove uint16
	targets := 0
	for row := range tetrisHeight {
		answerAbove |= answer[row]
		goal.protected[row] = answerAbove

		empty := tetrisFullRow &^ answer[row] &^ board[row]
		if clears[row] {
			if empty&(answerAbove|boardAbove) != 0 {
				return
			}
			goal.targets[row] = empty
			targets += bits.OnesCount16(empty)
		} else if answer[row] != 0 {
			if empty == 0 {
				return
			}
			goal.protected[row] |= 1 << randChoice(tetrisBits(empty))
		}
		boardAbove |= board[row]
	}

	// Too many cells to fill would make the stack overflow first.
	return goal, targets <= 4*lines
}

// The indices of the set bits.
func tetrisBits(mask uint16) (indices []int) {
	for ; mask != 0; mask &= mask - 1 {
		indices = append(indices, bits.TrailingZeros16(mask))
	}
	return
}

func tetrisTests() []test {
	// How many tests clear 1, 2, 3, and 4 lines.
	var tests []test
	for lines, count := range [5]int{0, 40, 25, 20, 15} {
		for range count {
			tests = append(tests, tetrisTest(lines))
		}
	}

	return shuffle(tests)
}

var _ = answerFunc("tetris", func() []Answer {
	return outputTests(tetrisTests())
})

// Accept any keys that clear as many lines as possible.
var _ = judge("tetris", perOutputJudge(func(game, userOutput, expected string) string {
	if game == "" {
		return expected
	}

	board, next := tetrisReplay(game)
	best, keys := tetrisBest(board, next)

	if strings.Trim(userOutput, "LRC") == "D" && strings.HasSuffix(userOutput, "D") &&
		board.play(next, userOutput) == best {
		return userOutput
	}

	if expected != "" {
		return expected
	}
	return keys
}))
