package main

import (
	"github.com/lillink13/yamusic-tui/api"
	"github.com/lillink13/yamusic-tui/cache"
	"github.com/lillink13/yamusic-tui/config"
	"github.com/lillink13/yamusic-tui/log"
	"github.com/lillink13/yamusic-tui/media"
	"github.com/lillink13/yamusic-tui/ui/model"
	loginpage "github.com/lillink13/yamusic-tui/ui/model/loginPage"
	mainpage "github.com/lillink13/yamusic-tui/ui/model/mainPage"
	"github.com/lillink13/yamusic-tui/ui/style"
)

func main() {
	log.Start()
	defer log.Stop()

	err := config.InitialLoad()
	if err != nil {
		log.Print(log.LVL_WARNIGN, "config load error: %s", err.Error())
	}

	style.Apply(config.Current.Style)
	api.SetupClient(config.Current.Proxy)
	cache.CleanPartial()

	if config.Current.Token == "" {
		err = loginpage.New().Run()
		if err != nil {
			log.Print(log.LVL_PANIC, err.Error())
			model.PrettyExit(err, 4)
		}
	}

	mediaHandler := media.NewHandler(config.DirName, "Yandex music terminal client")
	page := mainpage.New(mediaHandler)
	err = mediaHandler.Start(page.Run)
	if err != nil {
		log.Print(log.LVL_PANIC, err.Error())
		model.PrettyExit(err, 6)
	}
}
