package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jasonlvhit/gocron"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	yaml "gopkg.in/yaml.v2"
)

var (
	backupSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "backup_size",
		Help: "Current backup size.",
	})
	backupStatus = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "backup_status",
		Help: "Current backup status.",
	})
)

type Config struct {
	LogFile      string `yaml:"LogFile"`
	DocCommand   string `yaml:"DocCommand"`
	PgCommand    string `yaml:"PgCommand"`
	RsyncCommand string `yaml:"RsyncCommand"`
	Path         struct {
		BackupDir    string `yaml:"BackupDir"`
		DocBackupDir string `yaml:"DocBackupDir"`
	} `yaml:"Path"`
	BackUpServer struct {
		RemoteServer  string `yaml:"RemoteServer"`
		RemotePort    string `yaml:"RemotePort"`
		RemoteUser    string `yaml:"RemoteUser"`
		RemotePath    string `yaml:"RemotePath"`
		RemoteBwLimit string `yaml:"RemoteBwLimit"`
	} `yaml:"BackUpServer"`
	ShedulePlan struct {
		shPlan   string `yaml:"shPlan"`
		shTime   string `yaml:"shTime"`
		shHour   uint64 `yaml:"shHour"`
		shMinute uint64 `yaml:"shMinute"`
	} `yaml:"ShedulePlan"`
}

func init() {
	// Metrics have to be registered to be exposed:
	prometheus.MustRegister(backupSize)
	prometheus.MustRegister(backupStatus)
}

func createDump(docRun string) error {
	cmd := exec.Command("/bin/sh", "-c", docRun)
	cmd.Stderr = os.Stderr
	errCreateDump := cmd.Run()
	return errCreateDump
}

func syncDump(rsyncDump string) error {
	cmd := exec.Command("/bin/sh", "-c", rsyncDump)
	cmd.Stderr = os.Stderr
	errSyncDump := cmd.Run()
	return errSyncDump
}

func Dump(config Config) {
	var (
		timeStamp    string = time.Now().Format("2006-01-02")
		timeStampH   string = time.Now().Format("15-04")
		backupDir    string = config.Path.BackupDir + timeStamp + "/" + timeStampH + "/"
		docBackupDir string = config.Path.DocBackupDir + timeStamp + "/" + timeStampH + "/"
		docRun       string = config.DocCommand + "\"" + config.PgCommand + docBackupDir + "\""
		rsyncDump    string = config.RsyncCommand + "\"ssh -p " + config.BackUpServer.RemotePort + "\"" + backupDir + config.BackUpServer.RemoteUser + "@" + config.BackUpServer.RemoteServer + ":" + config.BackUpServer.RemotePath
	)
	os.Mkdir(config.Path.BackupDir, 0777)
	errCreateDump := createDump(docRun)
	if errCreateDump != nil {
		fmt.Printf("Снятие дампа закончилось с ошибкой: %v\r\n", errCreateDump)
		backupStatus.Set(1)
		os.Exit(1)
	}
	fmt.Printf("Снятие дампа успешно завершено\r\n")
	errSyncDump := syncDump(rsyncDump)
	if errSyncDump != nil {
		fmt.Printf("Синхронизация закончилась с ошибкой: %v\r\n", errSyncDump)
		backupStatus.Set(1)
		os.Exit(1)
	}
	fmt.Printf("Синхронизация дампа успешно завершена\r\n")
}

func main() {
	backupSize.Set(7000000)
	backupStatus.Set(0)
	filename, _ := filepath.Abs("./pg_backup.yml")
	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	var config Config
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
	var shplan string = config.ShedulePlan.shPlan
	shed := gocron.NewScheduler()
	if shplan == "time" {
		shed.Every(1).Day().At(config.ShedulePlan.shTime).Do(Dump, config)
	} else if shplan == "hour" {
		shed.Every(config.ShedulePlan.shHour).Hours().Do(Dump, config)
	} else if shplan == "minute" {
		shed.Every(config.ShedulePlan.shMinute).Minutes().Do(Dump, config)
	}

	sc := shed.Start()
	<-sc
	backupStatus.Set(0)
	// The Handler function provides a default handler to expose metrics
	// via an HTTP server. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe("127.0.0.1:48080", nil))
}
