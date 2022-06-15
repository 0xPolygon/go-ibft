package messages

import (
	"github.com/Trapesys/go-ibft/proto"
)

// messageHeap is a min-message heap (round number)
type messageHeap []*proto.Message

func (h *messageHeap) Len() int {
	return len(*h)
}

func (h *messageHeap) Less(i, j int) bool {
	left, right := (*h)[i].View, (*h)[j].View

	return left.Round < right.Round
}

func (h *messageHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
}

func (h *messageHeap) Push(x interface{}) {
	message, _ := x.(*proto.Message)
	*h = append(*h, message)
}

func (h *messageHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]

	return x
}
