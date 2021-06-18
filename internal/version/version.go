package version

import (
	"fmt"

	cli "github.com/jawher/mow.cli"
)

var (
	version = "Embedded Net DVR/NVR/DVS"
)

func Version() string {
	return version
}
func Print(cli *cli.Cmd) {
	fmt.Println("version = ", Version())
}
