package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/samber/lo"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/williamsjokvist/cfn-tracker/pkg/update"

	"github.com/williamsjokvist/cfn-tracker/cmd"
	"github.com/williamsjokvist/cfn-tracker/pkg/browser"
	"github.com/williamsjokvist/cfn-tracker/pkg/config"
	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	"github.com/williamsjokvist/cfn-tracker/pkg/server"
	cfgDb "github.com/williamsjokvist/cfn-tracker/pkg/storage/config"
	"github.com/williamsjokvist/cfn-tracker/pkg/storage/sql"
	"github.com/williamsjokvist/cfn-tracker/pkg/storage/txt"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/cfn"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/t8/wavu"
	"github.com/williamsjokvist/cfn-tracker/pkg/update/github"
)

// linker flags
const (
	capIDEmail    string = ""
	capIDPassword string = ""
	isProduction  string = ""
)

//go:embed all:gui/dist
var assets embed.FS

//go:embed gui/error/error.html
var errorTmpl []byte

//go:embed wails.json
var wailsJson []byte

var cfg config.BuildConfig
var logFile *os.File

var appBrowser *browser.Browser

func logToFile() {
	file, err := os.OpenFile("cfn-tracker.log", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Panic(err)
	}

	logFile = file
	log.SetOutput(file)
	log.SetFlags(log.Ldate | log.LstdFlags | log.Lshortfile)
}

type WailsJson struct {
	Info WailsJsonInfo `json:"info"`
}

type WailsJsonInfo struct {
	ProductVersion string `json:"productVersion"`
}

func init() {
	var wailsCfg WailsJson
	if err := json.Unmarshal(wailsJson, &wailsCfg); err != nil {
		log.Fatalf("parse project config: %v", err)
	}

	if isProduction == "true" {
		logToFile()
	}

	if err := loadEnvFile(envFilePath(), !isWailsBindings && !isGoTestProcess()); err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("load env file: %v", err)
		}
		cfg = config.BuildConfig{
			AppVersion:        wailsCfg.Info.ProductVersion,
			Headless:          isProduction == "true",
			CapIDEmail:        capIDEmail,
			CapIDPassword:     capIDPassword,
			BrowserSourcePort: 4242,
		}
		return
	}
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("process envconfig: %v", err)
	}
	cfg.AppVersion = wailsCfg.Info.ProductVersion
}

func loadEnvFile(path string, resolve1Password bool) error {
	values, err := godotenv.Read(path)
	if err != nil {
		return err
	}
	for key, value := range values {
		value = strings.TrimSpace(value)
		if resolve1Password && strings.HasPrefix(value, "op://") {
			resolved, err := read1PasswordSecret(value)
			if err != nil {
				return err
			}
			value = resolved
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}

func isGoTestProcess() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

func read1PasswordSecret(ref string) (string, error) {
	for {
		cmd := exec.Command("op", "read", ref)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err == nil {
			secret := strings.TrimSpace(string(out))
			if secret == "" {
				return "", fmt.Errorf("read 1Password reference %q: empty value", ref)
			}
			return secret, nil
		}
		detail := strings.TrimSpace(stderr.String())
		if isRetryable1PasswordError(detail) {
			slog.Warn("waiting for 1Password authorization", slog.String("ref", ref), slog.String("detail", detail))
			time.Sleep(3 * time.Second)
			continue
		}
		if detail != "" {
			return "", fmt.Errorf("read 1Password reference %q: %w: %s", ref, err, detail)
		}
		return "", fmt.Errorf("read 1Password reference %q: %w", ref, err)
	}
}

func isRetryable1PasswordError(detail string) bool {
	detail = strings.ToLower(detail)
	return strings.Contains(detail, "authorization timeout") ||
		strings.Contains(detail, "couldn't connect to the 1password desktop app") ||
		strings.Contains(detail, "could not connect to the 1password desktop app")
}

func envFilePath() string {
	if _, err := os.Stat("app/.env"); err == nil {
		return "app/.env"
	}
	return ".env"
}

func closeWithError(err error) {
	if appBrowser != nil {
		appBrowser.Page.Browser().Close()
	}
	slog.Error("closing with error", slog.Any("error", err))

	if err := wails.Run(&options.App{
		Title:                    "CFN Tracker - Error",
		Width:                    400,
		Height:                   148,
		DisableResize:            true,
		Frameless:                true,
		EnableDefaultContextMenu: false,
		AssetServer: &assetserver.Options{
			Middleware: assetserver.ChainMiddleware(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					var b bytes.Buffer
					tmpl := template.Must(template.New("errorPage").Parse(string(errorTmpl)))
					params := struct {
						Error string
					}{
						Error: err.Error(),
					}
					if err := tmpl.Execute(&b, params); err != nil {
						slog.Error("write error template", slog.Any("error", err))
					}
					_, err := w.Write(b.Bytes())
					if err != nil {
						slog.Error("write error page", slog.Any("error", err))
					}
				})
			}),
		},
	}); err != nil {
		slog.Error("display error window", slog.Any("error", err))
	}
	os.Exit(1)
}

func main() {
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	appBrowser, err := browser.NewBrowser(cfg.Headless)
	if err != nil {
		closeWithError(fmt.Errorf("launch browser: %w", err))
	}

	githubClient := github.NewClient()

	appVersion, err := version.NewVersion(cfg.AppVersion)
	if err != nil {
		closeWithError(fmt.Errorf("parse app version: %w", err))
	}
	lastRelease, err := githubClient.GetLastRelease()
	if err != nil {
		// maybe shouldn't close the app on this
		closeWithError(fmt.Errorf("parse latest app version: %w", err))
	}
	newUpdateAvailable := appVersion.LessThan(lastRelease.Version)
	if newUpdateAvailable {
		handleAutoUpdate(lastRelease.Version.Original())
		return
	}

	sqlDb, err := sql.NewStorage(false)
	if err != nil {
		closeWithError(fmt.Errorf("initalize database: %w", err))
	}
	noSqlDb, err := cfgDb.NewStorage()
	if err != nil {
		closeWithError(fmt.Errorf("initalize app config: %w", err))
	}
	txtDb, err := txt.NewStorage()
	if err != nil {
		closeWithError(fmt.Errorf("initalize text store: %w", err))
	}

	browserSrcMatchChan := make(chan model.Match, 1)
	cfnClient := cfn.NewClient(appBrowser)

	cmdHandler := cmd.NewCommandHandler(
		sqlDb,
		noSqlDb,
		txtDb,
		cfnClient,
		&cfg,
	)
	trackingHandler := cmd.NewTrackingHandler(
		wavu.NewClient(),
		cfnClient,
		sqlDb,
		noSqlDb,
		txtDb,
		&cfg,
		browserSrcMatchChan,
	)

	browserSrcServer := server.NewBrowserSourceServer(browserSrcMatchChan)

	var onSecondInstanceLaunch func(secondInstanceData options.SecondInstanceData)
	err = wails.Run(&options.App{
		Title:              fmt.Sprintf("CFN Tracker v%s", appVersion),
		Assets:             assets,
		Width:              1380,
		Height:             675,
		MinWidth:           800,
		MinHeight:          450,
		DisableResize:      false,
		Fullscreen:         false,
		Frameless:          true,
		StartHidden:        false,
		HideWindowOnClose:  false,
		LogLevel:           logger.WARNING,
		LogLevelProduction: logger.ERROR,
		ErrorFormatter:     model.FormatError,
		BackgroundColour:   options.NewRGBA(0, 0, 0, 1),
		CSSDragProperty:    "--draggable",
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			Theme:                             windows.Theme(windows.Dark),
			DisableFramelessWindowDecorations: true,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   fmt.Sprintf("CFN Tracker v%s", appVersion),
				Message: fmt.Sprintf("CFN Tracker version %s © 2025 William Sjökvist <william.sjokvist@gmail.com>", appVersion),
			},
		},
		OnDomReady: func(ctx context.Context) {
			onSecondInstanceLaunch = func(secondInstanceData options.SecondInstanceData) {
				slog.Warn("user opened second instance", slog.Any("args", strings.Join(secondInstanceData.Args, ",")))
				slog.Warn("user opened second from", slog.Any("working_directory", secondInstanceData.WorkingDirectory))
				runtime.WindowUnminimise(ctx)
				runtime.Show(ctx)
				go runtime.EventsEmit(ctx, "launchArgs", secondInstanceData.Args)
			}

			trackingHandler.SetEventEmitter(func(eventName string, optionalData ...interface{}) {
				slog.Debug("[FE]", slog.String("event", eventName), slog.Any("data", optionalData))
				runtime.EventsEmit(ctx, eventName, optionalData...)
			})
			cmdHandler.EventEmitter = func(eventName string, optionalData ...interface{}) {
				slog.Debug("[FE]", slog.String("event", eventName), slog.Any("data", optionalData))
				runtime.EventsEmit(ctx, eventName, optionalData...)
			}
		},
		OnStartup: func(ctx context.Context) {
			go browserSrcServer.Start(ctx, &cfg)
			go cmd.StartVegapunkSyncQueue(ctx, sqlDb)
			cmd.StartAutoDataRefresh(ctx, cmdHandler)
		},
		OnShutdown: func(_ context.Context) {
			appBrowser.Page.Browser().Close()
		},
		OnBeforeClose: func(_ context.Context) (prevent bool) {
			appBrowser.Page.Browser().Close()
			return false
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "d0ef6612-49f7-437a-9ffc-2076ec9e37db",
			OnSecondInstanceLaunch: onSecondInstanceLaunch,
		},
		Bind: []interface{}{
			cmdHandler,
			trackingHandler,
		},
		EnumBind: []interface{}{
			model.AllThemes,
			model.AllGameTypes,
			model.AllErrorKeys,
		},
	})
	if err != nil {
		closeWithError(fmt.Errorf("launch: %w", err))
	}
}

func handleAutoUpdate(versionStr string) {
	deleteOldExe := func() {
		slog.Info("deleting old exe...")
		currentExePath, err := os.Executable()
		if err != nil {
			slog.Error(fmt.Sprintf("get current exe path: %v", err))
		}

		err = os.Remove(currentExePath + ".old")
		if err != nil {
			slog.Error(fmt.Sprintf("remove current %s.old: %v", currentExePath, err))
		}

	}

	// If we have a previous instance running, wait for it to close before proceeding
	if i := lo.IndexOf(os.Args, "--auto-update"); i != -1 {
		if len(os.Args) < i+2 {
			panic("missing pid argument for --auto-update")
		}
		prevInstancePidStr := os.Args[i+1]
		prevInstancePid, err := strconv.Atoi(prevInstancePidStr)
		if err != nil {
			panic(fmt.Sprintf("convert pid to int: %v", err))
		}

		for i := 0; i < 10; i++ {

			// On Unix systems, FindProcess always succeeds and returns a Process
			// for the given pid, regardless of whether the process exists. To test whether
			// the process actually exists, see whether p.Signal(syscall.Signal(0)) reports
			// an error.
			p, err := os.FindProcess(prevInstancePid)
			if err != nil {
				slog.Warn(fmt.Sprintf("failed (err received) to find previous instance process, it's probably shut down...: %v'", err))
				deleteOldExe()
				break
			}
			if p == nil {
				slog.Info("find previous instance process, it's probably shut down...'")
				deleteOldExe()
				break
			}

			err = p.Signal(syscall.Signal(0))
			if err != nil {
				// The process is not running
				deleteOldExe()
				break
			}

			slog.Info("waiting for previous instance to close...")
			time.Sleep(1 * time.Second)

		}
	} else {
		err := update.HandleAutoUpdateTo(versionStr)
		if err != nil {
			slog.Error(fmt.Sprintf("update to latest version: %v", err))
		}
	}

}
