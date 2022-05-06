package ussh

import (
	_ "bytes"
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

var Num int

type Hander struct {
	Out     string
	session *ssh.Session
	client  *ssh.Client
	r       *ussh_r
	w       *ussh_w
	lock    *sync.Mutex
}

//shell命令输出通道
type ussh_r struct {
	c  chan string
	ct int
}

//shell命令输入通道
type ussh_w struct {
	c  chan string
	ct int
}

func new_r() *ussh_r {
	reader := new(ussh_r)
	reader.c = make(chan string)
	reader.ct = 0
	return reader
}

func new_w() *ussh_w {
	writer := new(ussh_w)
	writer.c = make(chan string)
	writer.ct = 0
	return writer
}

func (r *ussh_r) Read(p []byte) (n int, err error) {
	cmd := ""
	cmd = <-r.c
	bt := []byte(cmd + "\n")
	for i := 0; i < len(bt); i++ {
		p[i] = bt[i]
	}
	n = len(bt)
	return n, err
}

func (w *ussh_w) Write(p []byte) (n int, err error) {
	res := string(p)
	w.c <- res
	return len(p), err
}

func InitTerminal(IPAndPort string, user string, passwd string) (h Hander, err error) {
	client, err := ssh.Dial("tcp", IPAndPort, &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(passwd)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return h, err
	}

	session, err := client.NewSession()
	if err != nil {
		return h, err
	}

	w := new_w()
	r := new_r()
	session.Stdout = w
	session.Stderr = os.Stderr
	session.Stdin = r
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     //回显打开
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	term := "xterm"
	err = session.RequestPty(term, 180, 300, modes)
	if err != nil {
		return h, err
	}
	err = session.Shell()
	if err != nil {
		return h, err
	}
	h.Out = ""
	h.session = session
	h.client = client
	h.r = r
	h.w = w
	h.lock = new(sync.Mutex)
	err = h.GetOut(time.Second*6, true)
	return h, err
}

func (h *Hander) matchHead() bool {
	xp := regexp.MustCompile(`(.*?)@(.*?):((~#)|((.*?)#)|(~\$))`)
	return xp.MatchString(h.Out)
}

func (h *Hander) matchRootHead() bool {
	xp := regexp.MustCompile(`root@(.*?):((~#)|((.*?)#)|(~\$))`)
	return xp.MatchString(h.Out)
}

//获取输出命令行输出
func (h *Hander) GetOut(t time.Duration, c bool) error {
	success := make(chan bool)
	cherror := make(chan bool)
	//清空上次数据
	if c {
		h.ClearOut()
	}

	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()
	go func(ctx context.Context) {
		for {
			if h.matchHead() {
				success <- true
				return
			}
			time.Sleep(time.Millisecond * 100)
			str, ok := <-h.w.c
			if !ok {
				cherror <- true
				return
			}
			h.lock.Lock()
			h.Out += str
			h.lock.Unlock()
		}
	}(ctx)

	select {
	case <-ctx.Done():
		return nil
	case <-time.After(t + 1):
		if h.matchHead() {
			return nil
		}
		return errors.New("getout::get outstring timeout")
	case <-success:
		return nil
	case <-cherror:
		return errors.New("read chan error")
	}
}

func (h *Hander) ClearOut() {
	h.lock.Lock()
	h.Out = ""
	h.lock.Unlock()
}

func (h *Hander) WaitCmd(t time.Duration, key string) error {
	success := make(chan bool)
	cherror := make(chan bool)
	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()
	go func(ctx context.Context) {
		for {
			str, ok := <-h.w.c
			if !ok {
				cherror <- true
				return
			}
			time.Sleep(time.Millisecond * 100)
			h.lock.Lock()
			h.Out += str
			h.lock.Unlock()
			if strings.Contains(h.Out, key) {
				success <- true
				return
			}
		}
	}(ctx)
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(time.Duration(t + 1)):
		err := errors.New("WaitCmd ## Wait Cmd timeout!!!:" + key)
		return err
	case <-success:
		return nil
	}
}

func (h *Hander) GetOutStr() string {
	h.lock.Lock()
	ResStr := h.Out
	h.lock.Unlock()
	return ResStr
}

func (h *Hander) EndProsecces(root bool) {
	h.r.c <- "exit"
	if root {
		h.r.c <- "exit"
		h.r.c <- "exit"
	}
}

func (h *Hander) CloseHander() {
	h.session.Close()
	h.client.Close()
	close(h.r.c)
	close(h.w.c)
	h = nil
}

func (h *Hander) Exec(cmd string) {
	Slog.Info("Cmd:", cmd)
	h.r.c <- cmd
}

func (h *Hander) GetCmdOut(cmd string, t time.Duration) string {
	h.r.c <- cmd
	err := h.GetOut(t, false)
	if err != nil {
		return ""
	}
	return h.Out
}

func (h *Hander) GetSudoRoot(passwd string) error {
	h.ClearOut()
	h.Exec("sudo su")
	//不需要密码
	h.GetOut(time.Second, false)
	Slog.DebugOut(h.Out)
	if h.matchRootHead() {
		return nil //sudo su
	}
	//没有权限 sudo su 后不需要输入密码报权限问题
	err := h.WaitCmd(time.Second, "not in the sudoers file")
	if err == nil {
		return errors.New("sudo su:user no permission to run sudo su")
	}
	//需要密码
	if strings.Contains(h.Out, "[sudo] password for") {
		h.Exec(passwd)
		err = h.GetOut(time.Second*3, false) //可能存在的卡死 超时
		Slog.DebugOut(h.Out)
		if err != nil {
			if strings.Contains(h.Out, "try again") { //密码错误
				return errors.New("sudo su:passwd error")
			}
			if strings.Contains(h.Out, "not in the sudoers file") { //没有权限
				return errors.New("sudo su:user no permission to run sudo su")
			}
			return err
		}
		if h.matchRootHead() {
			return nil
		}
		return errors.New("sudo su:other errors")
	}
	Slog.DebugOut(h.Out)
	return nil
}

func (h *Hander) GetSuRoot(passwd string) error {
	h.ClearOut()
	h.Exec("su")
	err := h.WaitCmd(time.Second, "Password:") //su 一定需要密码
	if err != nil {
		return err
	}
	h.Exec(passwd)
	err = h.GetOut(time.Second*3, false)
	Slog.DebugOut(h.Out)
	if h.matchRootHead() {
		return nil //sudo su
	}
	if strings.Contains(h.Out, "fail") {
		return errors.New("su:passwd error")
	}

	if strings.Contains(err.Error(), "timeout") {
		return errors.New("su:login timeout")
	}

	return nil
}
