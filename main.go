package main

import (
	"context"
	"log"
	"os"

	cli "github.com/jawher/mow.cli"
	"github.com/lzh2nix/gb28181Simulator/internal/config"
	"github.com/lzh2nix/gb28181Simulator/internal/useragent"
	"github.com/qiniu/x/xlog"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	//xlog := xlog.NewWith(context.Background())
	xlog.SetOutputLevel(0)
	xlog.SetFlags(xlog.Llevel | xlog.Llongfile | xlog.Ltime)
	xlog := xlog.NewWith(context.Background())
	app := cli.App("gb28181Simulator", "Runs the gb28181 simulator.")
	//app.Spec = "[ -c=<configuration path> -v ] "
	//app.Spec = "[ -c=<configuration path> -v ] "
	confPath := app.StringOpt("c config", "sim.conf", "Specifies the configuration path (file) to use for the simulator.")
	detailLog := app.BoolOpt("v verbose", false, "Enables verbose logging.")
	id := app.StringOpt("i id", "", "Specifies the device id to use for the simulator.")
	app.Action = func() { run(xlog, app, confPath, *detailLog, *id) }

	// Register sub-commands
	//app.Command("version", "Prints the version of the executable.", version.Print)
	app.Run(os.Args)
}

func run(xlog *xlog.Logger, app *cli.Cli, conf *string, detailLog bool, id string) {
	xlog.Infof("gb28181 simulator is running...")
	cfg, err := config.ParseJsonConfig(conf)
	if err != nil {
		xlog.Errorf("load config file failed, err = ", err)
	}
	cfg.DetailLog = detailLog
	if id != "" {
		cfg.GBID = id
	}
	//xlog.Infof("config file = %#v", cfg)
	srv, err := useragent.NewService(xlog, cfg)
	if err != nil {
		xlog.Infof("new service failed err = %#v", err)
		return
	}
	srv.HandleIncommingMsg()
}
