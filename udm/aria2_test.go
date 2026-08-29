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

	// python3 -m http.server -d /
	d := udm.Job{

		// Out: fmt.Sprintf("download_%d.zip", time.Now().Unix()),
		Uri:              "http://127.0.0.1:8000/usr/lib/x86_64-linux-gnu/libc.a",
		Dir:              os.TempDir(),
		MaxDownloadLimit: "512K",
	}

	log.Println("Adding download")
	gid, err := a2.AddDownload(d)
	if err != nil {
		log.Println(err.Error())
		t.FailNow()
	}
	status := a2.Sub(gid)

	for s := range status {
		if s.Type == udm.DStatusMsg.Progress {
			fmt.Printf("Status: %.2f, %db/s\n", 100*float64(s.SizeLoaded)/float64(s.SizeTotal), s.Rate)
		}
	}

}

func Test_A2Error(t *testing.T) {

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Indexes do not match Error Code: %v", r)
		}
	}()

	for i, a2Err := range udm.A2Error {
		if i != a2Err.ErrCode {
			t.Fatalf("Indexes do not match Error Code: %v", i)
		}
	}
}
