package udm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	AppName      string
	DownloadDir  string
	CfgDir       string
	LogDir       string
	LogFile      string
	DbTimeLayout string

	Binary          string
	Aria2Port       int
	Secret          string
	StopWithApp     bool // --stop-with-process
	ExtraArgs       []string
	ReadyTimeout    time.Duration
	ShutdownTimeout time.Duration

	MsgPortStart int
	MsgPortEnd   int
}

func (c *Config) Defaults() {

	c.AppName = "udm_raffleberry"

	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	c.DownloadDir = filepath.Join(homeDir, "Downloads")
	err = os.MkdirAll(c.DownloadDir, 0755)
	if err != nil {
		panic(err)
	}

	c.CfgDir, err = os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	c.CfgDir = filepath.Join(c.CfgDir, c.AppName)

	err = os.MkdirAll(c.CfgDir, 0755)
	if err != nil {
		panic(err)
	}

	c.DbTimeLayout = "2006-01-02 15:04:05"

	c.LogDir = filepath.Join(c.CfgDir, "logs")

	logTimeLayout := time.Now().Format("2006_01_02_15_04_05")
	c.LogFile = filepath.Join(c.LogDir, fmt.Sprintf("udm_%s.log", time.Now().Format(logTimeLayout)))

	c.Aria2Port, err = FindFreePort(56000, 57000)
	if err != nil {
		panic(err)
	}

	c.MsgPortStart = 54000
	c.MsgPortEnd = 55000

	c.Secret = randomSecret()

	c.ReadyTimeout = 15 * time.Second

	c.ShutdownTimeout = 8 * time.Second

}

func NewConfig() *Config {
	c := &Config{}
	c.Defaults()
	return c
}
