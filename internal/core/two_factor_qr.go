package core

import (
	"errors"
	"fmt"
	"strings"
)

// A QR code for the otpauth:// address an authenticator app reads when
// two-factor sign-in is set up (the Rails core drew it with rqrcode):
// byte mode, error correction level M, versions 1 to 15 (up to 213 bytes,
// plenty for an otpauth address), drawn as an SVG.

// qrBlocks is each version's error correction at level M: EC codewords per
// block, then (blocks, data codewords) for each group.
var qrBlocks = [16][5]int{
	{},
	{10, 1, 16, 0, 0}, {16, 1, 28, 0, 0}, {26, 1, 44, 0, 0}, {18, 2, 32, 0, 0}, {24, 2, 43, 0, 0},
	{16, 4, 27, 0, 0}, {18, 4, 31, 0, 0}, {22, 2, 38, 2, 39}, {22, 3, 36, 2, 37}, {26, 4, 43, 1, 44},
	{30, 1, 50, 4, 51}, {22, 6, 36, 2, 37}, {22, 8, 37, 1, 38}, {24, 4, 40, 5, 41}, {24, 5, 41, 5, 42},
}

var qrAlign = [16][]int{
	{}, {}, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34}, {6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50},
	{6, 30, 54}, {6, 32, 58}, {6, 34, 62}, {6, 26, 46, 66}, {6, 26, 48, 70},
}

type qrCode struct {
	size     int
	modules  [][]bool
	function [][]bool
}

// qrEncode is text as a QR code.
func qrEncode(text string) (*qrCode, error) {
	data := []byte(text)
	version := 0
	for v := 1; v <= 15; v++ {
		b := qrBlocks[v]
		capacity := b[1]*b[2] + b[3]*b[4]
		countBits := 8
		if v >= 10 {
			countBits = 16
		}
		if 4+countBits+8*len(data) <= capacity*8 {
			version = v
			break
		}
	}
	if version == 0 {
		return nil, errors.New("too long for a QR code")
	}
	b := qrBlocks[version]
	capacity := b[1]*b[2] + b[3]*b[4]

	// The bit stream: byte mode, the count, the bytes, a terminator, pads.
	var bits []bool
	put := func(v, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (v>>i)&1 == 1)
		}
	}
	put(4, 4)
	if version >= 10 {
		put(len(data), 16)
	} else {
		put(len(data), 8)
	}
	for _, c := range data {
		put(int(c), 8)
	}
	put(0, min(4, capacity*8-len(bits)))
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	for pad := 0xEC; len(bits) < capacity*8; pad ^= 0xEC ^ 0x11 {
		put(pad, 8)
	}
	codewords := make([]byte, capacity)
	for i, bit := range bits {
		if bit {
			codewords[i/8] |= 1 << (7 - i%8)
		}
	}

	// Split into blocks, add each block's error correction, interleave.
	divisor := rsDivisor(b[0])
	var blocks, ecs [][]byte
	pos := 0
	for _, g := range [][2]int{{b[1], b[2]}, {b[3], b[4]}} {
		for range g[0] {
			block := codewords[pos : pos+g[1]]
			pos += g[1]
			blocks = append(blocks, block)
			ecs = append(ecs, rsRemainder(block, divisor))
		}
	}
	var final []byte
	for i := 0; i < max(b[2], b[4]); i++ {
		for _, block := range blocks {
			if i < len(block) {
				final = append(final, block[i])
			}
		}
	}
	for i := 0; i < b[0]; i++ {
		for _, ec := range ecs {
			final = append(final, ec[i])
		}
	}

	q := &qrCode{size: version*4 + 17}
	q.modules = make([][]bool, q.size)
	q.function = make([][]bool, q.size)
	for i := range q.modules {
		q.modules[i] = make([]bool, q.size)
		q.function[i] = make([]bool, q.size)
	}
	q.drawFunctions(version)
	q.drawCodewords(final)

	// The mask with the lowest penalty.
	best, bestScore := 0, -1
	for mask := range 8 {
		q.applyMask(mask)
		q.drawFormat(mask)
		if score := q.penalty(); bestScore < 0 || score < bestScore {
			best, bestScore = mask, score
		}
		q.applyMask(mask) // XOR undoes it
	}
	q.applyMask(best)
	q.drawFormat(best)
	return q, nil
}

func (q *qrCode) set(x, y int, dark bool) {
	q.modules[y][x] = dark
	q.function[y][x] = true
}

func (q *qrCode) drawFunctions(version int) {
	for i := range q.size {
		q.set(6, i, i%2 == 0)
		q.set(i, 6, i%2 == 0)
	}
	for _, c := range [][2]int{{3, 3}, {q.size - 4, 3}, {3, q.size - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := c[0]+dx, c[1]+dy
				if x < 0 || y < 0 || x >= q.size || y >= q.size {
					continue
				}
				d := max(qrAbs(dx), qrAbs(dy))
				q.set(x, y, d != 2 && d != 4)
			}
		}
	}
	pos := qrAlign[version]
	for i, ay := range pos {
		for j, ax := range pos {
			if (i == 0 && j == 0) || (i == 0 && j == len(pos)-1) || (i == len(pos)-1 && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					q.set(ax+dx, ay+dy, max(qrAbs(dx), qrAbs(dy)) != 1)
				}
			}
		}
	}
	q.drawFormat(0) // reserves the format areas; redrawn with the mask
	if version >= 7 {
		rem := version
		for range 12 {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		bits := version<<12 | rem
		for i := range 18 {
			dark := (bits>>i)&1 == 1
			a, b := q.size-11+i%3, i/3
			q.set(a, b, dark)
			q.set(b, a, dark)
		}
	}
}

// drawFormat writes level M's format bits for mask, both copies, and the
// dark module.
func (q *qrCode) drawFormat(mask int) {
	data := 0<<3 | mask // M is 00
	rem := data
	for range 10 {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	for i := 0; i <= 5; i++ {
		q.set(8, i, bit(i))
	}
	q.set(8, 7, bit(6))
	q.set(8, 8, bit(7))
	q.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		q.set(14-i, 8, bit(i))
	}
	for i := range 8 {
		q.set(q.size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		q.set(8, q.size-15+i, bit(i))
	}
	q.set(8, q.size-8, true)
}

func (q *qrCode) drawCodewords(data []byte) {
	i := 0
	for right := q.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := range q.size {
			for j := range 2 {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = q.size - 1 - vert
				}
				if !q.function[y][x] && i < len(data)*8 {
					q.modules[y][x] = (data[i>>3]>>(7-i&7))&1 == 1
					i++
				}
			}
		}
	}
}

func (q *qrCode) applyMask(mask int) {
	for y := range q.size {
		for x := range q.size {
			if q.function[y][x] {
				continue
			}
			var flip bool
			switch mask {
			case 0:
				flip = (x+y)%2 == 0
			case 1:
				flip = y%2 == 0
			case 2:
				flip = x%3 == 0
			case 3:
				flip = (x+y)%3 == 0
			case 4:
				flip = (x/3+y/2)%2 == 0
			case 5:
				flip = x*y%2+x*y%3 == 0
			case 6:
				flip = (x*y%2+x*y%3)%2 == 0
			case 7:
				flip = ((x+y)%2+x*y%3)%2 == 0
			}
			if flip {
				q.modules[y][x] = !q.modules[y][x]
			}
		}
	}
}

// penalty scores runs, blocks and imbalance (the spec's rules 1, 2 and 4):
// enough to keep a mask that reads badly from being chosen.
func (q *qrCode) penalty() int {
	score, dark := 0, 0
	for y := range q.size {
		for _, horizontal := range []bool{true, false} {
			run, prev := 0, false
			for x := range q.size {
				m := q.modules[y][x]
				if !horizontal {
					m = q.modules[x][y]
				}
				if x > 0 && m == prev {
					run++
					if run == 5 {
						score += 3
					} else if run > 5 {
						score++
					}
				} else {
					run, prev = 1, m
				}
			}
		}
		for x := range q.size {
			if q.modules[y][x] {
				dark++
			}
			if x < q.size-1 && y < q.size-1 {
				c := q.modules[y][x]
				if c == q.modules[y][x+1] && c == q.modules[y+1][x] && c == q.modules[y+1][x+1] {
					score += 3
				}
			}
		}
	}
	total := q.size * q.size
	score += qrAbs(dark*20-total*10) / total * 10
	return score
}

// svg draws the code with a quiet zone of four modules.
func (q *qrCode) svg() string {
	n := q.size + 8
	var path strings.Builder
	for y := range q.size {
		for x := range q.size {
			if q.modules[y][x] {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x+4, y+4)
			}
		}
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="QR code for your authenticator app"><rect width="100%%" height="100%%" fill="#fff"/><path fill="#000" d="%s"/></svg>`, n, n, path.String())
}

func rsDivisor(degree int) []byte {
	result := make([]byte, degree)
	result[degree-1] = 1
	root := byte(1)
	for range degree {
		for j := range result {
			result[j] = gfMul(result[j], root)
			if j+1 < len(result) {
				result[j] ^= result[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	return result
}

func rsRemainder(data, divisor []byte) []byte {
	result := make([]byte, len(divisor))
	for _, b := range data {
		factor := b ^ result[0]
		copy(result, result[1:])
		result[len(result)-1] = 0
		for i := range result {
			result[i] ^= gfMul(divisor[i], factor)
		}
	}
	return result
}

func gfMul(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= ((int(y) >> i) & 1) * int(x)
	}
	return byte(z)
}

func qrAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
