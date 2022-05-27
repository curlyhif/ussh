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
	Name    string
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
	cmd, ok := <-r.c
	if !ok {
		return 0, errors.New("read error")
	}
	bt := []byte(cmd + "\n")
	for i := 0; i < len(bt); i++ {
		p[i] = bt[i]
	}
	n = len(bt)
	return n, err
}

func (w *ussh_w) Write(p []byte) (n int, err error) {
	success := make(chan bool)
	chclose := make(chan bool)
	chpanic := make(chan bool)
	res := string(p)
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	wg.Add(1)
	go func(ctx context.Context) {
		defer func() {
			if r := recover(); r != nil {
				chpanic <- true
				wg.Done()
				return
			}
		}()
		select {
		case w.c <- res:
			wg.Done()
			success <- true
			return
		case <-chclose:
			wg.Done()
			return
		}
	}(ctx)
	select {
	case <-ctx.Done():
		Slog.Error("命令输入超时")
		chclose <- true
		wg.Wait()
		return 0, errors.New("write cmd timeout")
	case <-success:
		wg.Wait()
		return len(p), err
	case <-chpanic:
		Slog.Error("网络不稳定导致端口关闭")
		wg.Wait()
		return 0, errors.New("write panic")
	}
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
		ssh.ECHO:          0,     //回显关闭
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
	h.Name = IPAndPort
	h.session = session
	h.client = client
	h.r = r
	h.w = w
	h.lock = new(sync.Mutex)
	err = h.getOut(time.Second*6, true, "Login")
	return h, err
}

func (h *Hander) matchHead() bool {
	xp := regexp.MustCompile(`(.*?)@(.*?):((~#)|((.*?)#)|(~\$))`)
	res := h.GetOutStr()
	return xp.MatchString(res)
}

func (h *Hander) MatchStr(sub string, s string) string {
	xp := regexp.MustCompile(s)
	return xp.FindString(sub)
}

func (h *Hander) matchRootHead() bool {
	xp := regexp.MustCompile(`root@(.*?):((~#)|((.*?)#)|(~\$))`)
	res := h.GetOutStr()
	return xp.MatchString(res)
}

//获取输出命令行输出
func (h *Hander) getOut(t time.Duration, c bool, cmd string) error {
	var wg sync.WaitGroup
	success := make(chan bool)
	chclose := make(chan bool)
	chpanic := make(chan bool)
	//清空上次数据
	if c {
		h.ClearOut()
	}
	wg.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()
	go func(ctx context.Context) {
		defer func() {
			if r := recover(); r != nil {
				chpanic <- true
				Slog.Error("奔溃了")
				wg.Done()
				return
			}
		}()
		for {
			select {
			case <-chclose:
				wg.Done()
				return
			case str, ok := <-h.w.c:
				if !ok { // 通道读通道关闭
					chpanic <- true
					Slog.Error("奔溃了")
					wg.Done()
					return
				}
				h.lock.Lock()
				h.Out += str
				h.lock.Unlock()
				if h.matchHead() {
					wg.Done()
					success <- true
					return
				}
			default:
				time.Sleep(time.Millisecond * 200)
			}
		}
	}(ctx)

	select {
	case <-ctx.Done():
		chclose <- true
		wg.Wait()
		if h.matchHead() {
			return nil
		}
		Slog.ErrorOut("getout::[" + cmd + "]\n" + h.Out)
		return errors.New("getout::get outstring timeout # " + cmd)
	case <-success:
		wg.Wait()
		return nil
	case <-chpanic:
		wg.Wait()
		return errors.New("getout::panic # " + cmd)
	}
}

func (h *Hander) ClearOut() {
	h.lock.Lock()
	h.Out = ""
	h.lock.Unlock()
}

func (h *Hander) WaitCmd(t time.Duration, key string) error {
	var wg sync.WaitGroup
	success := make(chan bool)
	chclose := make(chan bool)
	chpanic := make(chan bool)
	wg.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()
	go func(ctx context.Context) {
		defer func() {
			if r := recover(); r != nil {
				Slog.Error("奔溃了")
				chpanic <- true
				wg.Done()
				return
			}
		}()
		for {
			select {
			case <-chclose:
				wg.Done()
				return
			case str, ok := <-h.w.c:
				if !ok { // 通道读通道关闭
					chpanic <- true
					Slog.Error("奔溃了")
					wg.Done()
					return
				}
				h.lock.Lock()
				h.Out += str
				h.lock.Unlock()
				if strings.Contains(h.Out, key) {
					wg.Done()
					success <- true
					return
				}
			default:
				time.Sleep(time.Millisecond * 200)
			}
		}
	}(ctx)
	select {
	case <-ctx.Done():
		chclose <- true
		wg.Wait()
		if strings.Contains(h.Out, key) {
			return nil
		}
		Slog.ErrorOut("not find [" + key + "]\n" + h.Out)
		return errors.New("waitcmd::get outstring timeout # " + key)
	case <-success:
		wg.Wait()
		return nil
	case <-chpanic:
		wg.Wait()
		return errors.New("waitcmd::panic # " + key)
	}
}

func (h *Hander) GetOutStr() string {
	h.lock.Lock()
	ResStr := h.Out
	h.lock.Unlock()
	return ResStr
}

func (h *Hander) GetErrorMsg() string {
	s := h.GetOutStr()
	xp := regexp.MustCompile(`(.*?)@(.*?):((~#)|((.*?)#)|(~\$))`)
	iList := xp.FindStringIndex(s)
	if iList == nil {
		return s
	}
	s = s[:iList[0]] + s[iList[1]:] //iList长度一定为2
	sList := strings.Split(s, "\n")
	return sList[0]
}

func (h *Hander) EndProsecces(root bool) {
	h.r.c <- "exit"
	if root {
		h.r.c <- "exit"
		h.r.c <- "exit"
	}
}

func (h *Hander) CloseHander(debug string) {
	h.EndProsecces(true)
	Slog.Debug("###########:", debug, " 读写通道被关闭")
	h.session.Close()
	h.client.Close()
	close(h.r.c)
	close(h.w.c)
	h = nil
}

func (h *Hander) Close(debug string) {
	Slog.Debug("###########:", debug, " 读写通道被关闭")
	h.session.Close()
	h.client.Close()
	close(h.r.c)
	close(h.w.c)
	h = nil
}

func (h *Hander) Exec(cmd string) error {
	Slog.Info("Cmd:", cmd)
	select {
	case h.r.c <- cmd:
		Slog.Info("Cmd Run")
		return nil
	default:
		Slog.Error("not read cmd")
		return errors.New("not read cmd")
	}
}

//以下相关涉及linux shell xterm 相关性质

//执行有输出的cmd命令
func (h *Hander) GetCmdOut(cmd string, t time.Duration) string {
	h.ClearOut()
	if len(cmd) == 0 {
		return "cmd empty"
	}
	err := h.Exec(cmd)
	if err != nil {
		return err.Error()
	}
	err = h.getOut(t, false, cmd)
	if err != nil {
		Slog.Error(err)
		Slog.Error(h.GetOutStr())
		return ""
	}
	return h.GetOutStr()
}

//执行cmd命令
func (h *Hander) RunCmd(cmd string, t time.Duration) error {
	h.ClearOut()
	if len(cmd) == 0 {
		return errors.New("cmd empty")
	}
	err := h.Exec(cmd)
	if err != nil {
		return err
	}
	err = h.getOut(t, false, cmd)
	if err != nil {
		Slog.Error(err)
		return err
	}
	return nil
}

//执行cmd命令带校验
func (h *Hander) RunCmdErr(cmd string, t time.Duration) error {
	h.ClearOut()
	if len(cmd) == 0 {
		return errors.New("cmd empty")
	}
	err := h.Exec(cmd)
	if err != nil {
		return err
	}
	err = h.getOut(t, false, cmd)
	if err != nil {
		Slog.Error(err)
		return err
	}
	cmdList := strings.Split(cmd, " ")
	if strings.Contains(h.GetOutStr(), cmdList[0]) {
		msg := h.GetErrorMsg()
		Slog.ErrorOut("RunCmdErr:" + msg)
		return errors.New(msg)
	}
	return nil
}

//执行普通交互命令
func (h *Hander) GetCross(cmd string, key string, t time.Duration) (string, error) {
	h.ClearOut()
	if len(cmd) == 0 {
		return "cmd empty", errors.New("cmd empty")
	}
	err := h.Exec(cmd)
	if err != nil {
		return "", err
	}
	err = h.WaitCmd(t, key) //su 一定需要密码
	if err != nil {
		return h.GetOutStr(), err
	}
	return h.GetOutStr(), nil
}

func (h *Hander) GetSudoRoot(passwd string) error {
	h.ClearOut()
	h.Exec("sudo su")
	//不需要密码
	h.getOut(time.Second, false, "sudo su")
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
		err = h.getOut(time.Second*3, false, "password") //可能存在的卡死 超时
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
	err = h.getOut(time.Second*3, false, "su")
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
