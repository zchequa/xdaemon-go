package xdaemon-go

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

const EnvName = "ENV_NAME_XF"

var callCount int = 0

type Daemon struct {
	logFile         string // 日志文件路径，空则不记录
	maxErrorRestart int    // 子进程异常退出次数 超过则守护进程退出
	exitTime        int64  // 子进程正常退出时长 低于该值则表示子进程异常退出  单位：秒
}

func NewDaemon(logFile string) *Daemon {
	return &Daemon{
		logFile:         logFile,
		maxErrorRestart: 5,
		exitTime:        10,
	}
}

func (d *Daemon) SetErrorRestartCount(count int) {
	d.maxErrorRestart = count
}

func (d *Daemon) Run(pidFile string) {
	backend(d.logFile, pidFile, true)

	var t int64
	errNum := 0

	for {
		if errNum > d.maxErrorRestart {
			log.Errorf("start son process too many times, exit")
			os.Exit(1)
		}

		t = time.Now().Unix()
		cmd, err := backend(d.logFile, pidFile, false)
		if err != nil {
			errNum++
			continue
		}
		if cmd == nil {
			break
		}
		_ = writePidFile(pidFile, os.Getpid(), cmd.Process.Pid)
		err = cmd.Wait()
		runningTime := time.Now().Unix() - t
		if runningTime < d.exitTime {
			errNum++
		} else {
			errNum = 0
		}
		log.Printf("son process %d total running %d seconds", cmd.Process.Pid, runningTime)
	}
}

func startProc(args, env []string, logFile, pidFile string) (*exec.Cmd, error) {
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		array, err := readPidFile(pidFile)
		if err != nil {
			os.Exit(1)
		}
		pid, _ := strconv.Atoi(array[1]) // 子进程
		process, _ := os.FindProcess(pid)
		if isRunning(process) {
			log.Warningf("son process is running")
			os.Exit(0)
		}
	}

	cmd := &exec.Cmd{
		Path:        os.Args[0],
		Args:        args,
		Env:         env,
		SysProcAttr: &syscall.SysProcAttr{Setsid: true}, // 子进程设置会话ID
	}
	if logFile != "" {
		out, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		cmd.Stdout = out
		cmd.Stderr = out
	}
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func backend(logFile, pidFile string, isExist bool) (*exec.Cmd, error) {
	callCount++

	envVal, err := strconv.Atoi(os.Getenv(EnvName))
	if err != nil {
		envVal = 0
	}

	if callCount <= envVal {
		return nil, nil
	}

	env := os.Environ()
	env = append(env, fmt.Sprintf("%s=%d", EnvName, callCount))

	cmd, err := startProc(os.Args, env, logFile, pidFile)
	if err != nil {
		return nil, err
	}

	if isExist {
		os.Exit(0)
	}

	return cmd, nil
}

func writePidFile(pidFile string, fPid, sPid int) error {
	const (
		FilePerm = 0666
	)
	file, err := os.OpenFile(pidFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, FilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	pid := fmt.Sprintf("%d_%d", fPid, sPid)
	if _, err = file.Write([]byte(pid)); err != nil {
		return err
	}
	file.Sync()
	return nil
}

func readPidFile(pidFile string) ([]string, error) {
	const (
		ValidCount = 2
	)
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return nil, err
	}
	str := strings.Split(strings.TrimSpace(string(data)), "\n")
	array := strings.Split(str[0], "_")
	if len(array) != ValidCount {
		return nil, fmt.Errorf("invalid content of pid file")
	}
	return array, nil
}

func isRunning(process *os.Process) bool {
	if err := process.Signal(syscall.Signal(0)); err == nil {
		return true
	} else {
		return false
	}
}
