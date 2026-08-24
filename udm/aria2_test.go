package udm_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/raffleberry/udm/udm"
	"github.com/stretchr/testify/require"
)

func Test_filepathSymlinks(t *testing.T) {
	p, err := filepath.EvalSymlinks("/home/user/Code/udm/udm/aria2_test.go")
	t.Logf("p:[%v], err:[%v]", p, err)
}

func Test_AddDownload(t *testing.T) {
	cfg := udm.NewConfig()
	cfg.Defaults()
	a2 := udm.NewA2(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := a2.Start(ctx)
	cancel()
	require.Nil(t, err)

	d := udm.Download{
		FileName:         fmt.Sprintf("udm_%d.zip", time.Now().Unix()),
		Uri:              "https://github.com/raffleberry/udm/archive/refs/heads/main.zip",
		MaxDownloadLimit: "50K",
		Dir:              os.TempDir(),
	}
	log.Println("Adding download")
	err, done := a2.AddDownload(d)
	if err != nil {
		log.Println(err.Error())
		t.FailNow()
	}

	require.Nil(t, err)
	<-done
	a2.Shutdown(context.Background())

}
