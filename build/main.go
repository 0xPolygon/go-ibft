package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/0xPolygon/go-ibft/core"
)

func main() {
	b := core.NewIBFT(nil, nil, nil)

	// prevent golang compiler from removing the whole function
	io.Copy(io.Discard, strings.NewReader(fmt.Sprint(b)))
}
