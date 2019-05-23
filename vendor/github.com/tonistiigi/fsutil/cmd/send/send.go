package main

import (
	"flag"
	"os"

	"github.com/tonistiigi/fsutil"
	"github.com/tonistiigi/fsutil/util"
	"golang.org/x/net/context"
)

func main() {
	flag.Parse()
	if len(flag.Args()) == 0 {
		panic("source path not set")
	}

	s := util.NewProtoStream(os.Stdin, os.Stdout)

	if err := fsutil.Send(context.Background(), s, flag.Args()[0], nil, nil); err != nil {
		panic(err)
	}
}
