package udm_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/raffleberry/udm/udm"
	"github.com/stretchr/testify/require"
)

func Test_filepathSymlinks(t *testing.T) {
	if runtime.GOOS == "linux" {
		p, err := filepath.EvalSymlinks("/home/user/Code/udm/udm/aria2_test.go")
		t.Logf("p:[%v], err:[%v]", p, err)
	}
}

func Test_AddDownload(t *testing.T) {
	cfg := udm.NewConfig()
	cfg.Defaults()
	a2 := udm.NewA2(cfg)
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Second*5))
	defer cancel()

	err := a2.Start(ctx)
	require.Nil(t, err)
	defer a2.Shutdown(context.Background())

	d := udm.Download{
		Out: fmt.Sprintf("udm_%d.zip", time.Now().Unix()),
		Uri: "https://github.com/raffleberry/udm/archive/refs/heads/main.zip",
		Dir: os.TempDir(),
	}
	log.Println("Adding download")
	gid, err, status := a2.AddDownload(d)
	if err != nil {
		log.Println(err.Error())
		t.FailNow()
	}
	a2.AddPollFor(gid)

	for s := range status {
		fmt.Println("Status Chan", s)
	}

}
