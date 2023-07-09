package search

import (
	"bytes"

	"github.com/ollybritton/StupidChess/position"
)

type pvList []position.Move

func (pv *pvList) new() {
	*pv = make(pvList, 0, 100) // TODO: Replace 100 with max ply
}

func (pv *pvList) add(mv position.Move) {
	*pv = append(*pv, mv)
}

func (pv *pvList) clear() {
	*pv = (*pv)[:0]
}

func (pv *pvList) addPV(pv2 *pvList) {
	*pv = append(*pv, *pv2...)
}

func (pv *pvList) catenate(mv position.Move, pv2 *pvList) {
	pv.clear()
	pv.add(mv)
	pv.addPV(pv2)
}

func (pv *pvList) String() string {
	var out bytes.Buffer
	var length = len(*pv)

	for i, move := range *pv {
		out.WriteString(move.String())

		if i != length-1 {
			out.WriteString(" ")
		}
	}

	return out.String()
}
