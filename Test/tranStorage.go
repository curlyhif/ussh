package Test

import (
	"cmdexec/ussh"
	"errors"
	"strings"
	"time"
)

type TranSto struct {
	Port    string
	IP      string
	TIp     string
	TPort   string
	User    string
	Passwd  string
	NewUuid string
	OldUuid string
}

func TranUuidsto(tran TranSto) error {
	//获取原服务器和目标服务器session hander
	ter, err := GetHander(tran.IP+":"+tran.Port, tran.User, tran.Passwd)
	if err != nil {
		return err
	}
	defer EndHander(ter)
	tagter, err := GetHander(tran.TIp+":"+tran.TPort, tran.User, tran.Passwd)
	if err != nil {
		return err
	}
	defer EndHander(tagter)

	//查看原存储和目标存储是否被挂载
	err = CheckSto(ter, tagter, tran.NewUuid, tran.OldUuid)
	if err != nil {
		return err
	}
	//defer 清理主流程的 文件、目录、 分区挂载
	defer ClearFileAndUmount(ter, tagter, tran.NewUuid, tran.OldUuid)
	//############ 主流程
	err = PackUuidSto(ter, tran.OldUuid)
	if err != nil {
		return err
	}
	err = ScpUuidSto(ter, tran.TIp, tran.TPort, tran.OldUuid, tran.User, tran.Passwd)
	if err != nil {
		return err
	}
	err = ReleaseUuidSto(tagter, tran.NewUuid, tran.OldUuid)
	if err != nil {
		return err
	}
	//############ 主流程 完成
	return nil
}

func GetHander(iport string, u string, p string) (*ussh.Hander, error) {
	ter, err := ussh.InitTerminal(iport, u, p)
	if err != nil {
		return &ter, err
	}
	err = ter.GetSudoRoot(p)
	if err != nil {
		return &ter, err
	}
	return &ter, err
}

func EndHander(ter *ussh.Hander) {
	ter.CloseHander("")
}

func PackUuidSto(ter *ussh.Hander, oldUuid string) error {
	blkOut := ter.GetCmdOut("blkid | grep "+oldUuid+" | awk -F: '{print $1}'", time.Second*5)
	dev := ter.MatchStr(blkOut, `/dev/sd[a-z]{1}[0-9]{1,}`)
	//fmt.Println("[" + dev + "]")

	err := ter.RunCmd("mkdir /home/ops_manager/mnt_"+oldUuid, time.Second*2)
	if err != nil {
		return err
	}

	err = ter.RunCmdErr("mount "+dev+" /home/ops_manager/mnt_"+oldUuid, time.Second*2)
	if err != nil {
		return err
	}

	err = ter.RunCmd(
		"tar -cpf /home/ops_manager/package_"+oldUuid+".tgz -C /home/ops_manager/mnt_"+oldUuid+" .",
		time.Minute*2)
	if err != nil {
		return err
	}
	return nil
}

func ScpUuidSto(ter *ussh.Hander, tip string, tport string, oldUuid string, u string, p string) error {
	scpStr, _ := ter.GetCross("scp -P "+tport+" /home/ops_manager/package_"+oldUuid+".tgz "+
		u+"@"+tip+":", "yes/no", time.Second*3)
	if strings.Contains(scpStr, "yes/no") {
		yesStr, _ := ter.GetCross("yes", "password:", time.Second*2)
		if strings.Contains(yesStr, "password:") {
			err := ter.RunCmd(p, time.Minute)
			if err != nil {
				return err
			}
		}
	}
	if strings.Contains(scpStr, "password:") {
		err := ter.RunCmd(p, time.Minute)
		if err != nil {
			return err
		}
	}

	if strings.Contains(ter.GetOutStr(), "try again") {
		return errors.New("passwd error")
	}

	if strings.Contains(ter.GetOutStr(), "100%") {
		return nil
	}
	return errors.New(ter.GetOutStr())
}

func ReleaseUuidSto(ter *ussh.Hander, newUuid string, oldUuid string) error {
	blkOut := ter.GetCmdOut("blkid | grep "+newUuid+" | awk -F: '{print $1}'", time.Second*3)
	dev := ter.MatchStr(blkOut, `/dev/sd[a-z]{1}[0-9]{1,}`)
	//fmt.Println("[" + dev + "]")

	err := ter.RunCmd("mkfs.ext4 "+dev+" -F", time.Second*5)
	if err != nil {
		return err
	}

	err = ter.RunCmd("mkdir /home/ops_manager/mnt_"+oldUuid, time.Second*2)
	if err != nil {
		return err
	}

	err = ter.RunCmdErr("mount "+dev+" /home/ops_manager/mnt_"+oldUuid, time.Second*3)
	if err != nil {
		return err
	}

	err = ter.RunCmd(
		"tar -xf /home/ops_manager/package_"+oldUuid+".tgz -C /home/ops_manager/mnt_"+oldUuid,
		time.Minute*2)
	if err != nil {
		return err
	}

	return nil
}

func CheckSto(ter *ussh.Hander, tter *ussh.Hander, newUuid string, oldUuid string) error {
	blkOut := ter.GetCmdOut("blkid | grep "+oldUuid+" | awk -F: '{print $1}'", time.Second*3)
	dev := ter.MatchStr(blkOut, `sd[a-z]{1}[0-9]{1,}`)

	tblkOut := tter.GetCmdOut("blkid | grep "+newUuid+" | awk -F: '{print $1}'", time.Second*3)
	tdev := tter.MatchStr(tblkOut, `sd[a-z]{1}[0-9]{1,}`)

	res := ter.GetCmdOut("lsblk | grep \""+dev+"\"", time.Second*3)
	tres := tter.GetCmdOut("lsblk | grep \""+tdev+"\"", time.Second*3)

	if !strings.Contains(res, dev) {
		return errors.New(oldUuid + "[" + dev + "] does not exist on the original server")
	}
	if !strings.Contains(tres, tdev) {
		return errors.New(newUuid + "[" + tdev + "] does not exist on the original server")
	}

	if strings.Contains(res, "/") {
		return errors.New(oldUuid + "[" + dev + "] has been mounted")
	}
	if strings.Contains(tres, "/") {
		return errors.New(newUuid + "[" + tdev + "] has been mounted")
	}
	return nil
}

//清理
func ClearFileAndUmount(ter *ussh.Hander, tter *ussh.Hander, newUuid string, oldUuid string) error {
	var err error
	blkOut := ter.GetCmdOut("blkid | grep "+oldUuid+" | awk -F: '{print $1}'", time.Second*3)
	dev := ter.MatchStr(blkOut, `sd[a-z]{1}[0-9]{1,}`)
	tblkOut := tter.GetCmdOut("blkid | grep "+newUuid+" | awk -F: '{print $1}'", time.Second*3)
	tdev := tter.MatchStr(tblkOut, `sd[a-z]{1}[0-9]{1,}`)

	res := ter.GetCmdOut("lsblk | grep \""+dev+"\"", time.Second*3)
	tres := tter.GetCmdOut("lsblk | grep \""+tdev+"\"", time.Second*3)
	if strings.Contains(res, "ops_manager/mnt_") {
		err = ter.RunCmdErr("umount /home/ops_manager/mnt_"+oldUuid, time.Second*3)
		if err != nil {
			ussh.Slog.Error(err)
		}
	}
	if strings.Contains(tres, "ops_manager/mnt_") {
		err = tter.RunCmdErr("umount /home/ops_manager/mnt_"+oldUuid, time.Second*3)
		if err != nil {
			ussh.Slog.Error(err)
		}
	}

	lslOut := ter.GetCmdOut("ls -l | grep "+oldUuid, time.Second*3)
	if strings.Contains(lslOut, "mnt_") {
		err = ter.RunCmdErr("rm -r /home/ops_manager/mnt_"+oldUuid, time.Second*3)
		if err != nil {
			ussh.Slog.Error(err)
		}
	}
	if strings.Contains(lslOut, "package_") {
		err = ter.RunCmdErr("rm /home/ops_manager/package_"+oldUuid+".tgz", time.Second*3)
		if err != nil {
			ussh.Slog.Error(err)
		}
	}

	tlsOut := tter.GetCmdOut("ls -l | grep "+oldUuid, time.Second*3)
	if strings.Contains(tlsOut, "mnt_") {
		err = tter.RunCmdErr("rm -r /home/ops_manager/mnt_"+oldUuid, time.Second*3)
		if err != nil {
			ussh.Slog.Error(err)
		}
	}
	if strings.Contains(tlsOut, "package_") {
		err = tter.RunCmdErr("rm /home/ops_manager/package_"+oldUuid+".tgz", time.Second*3)
		if err != nil {
			ussh.Slog.Error(err)
		}
	}
	return nil
}

//格式化存储
func ClearSto(ter *ussh.Hander, oldUuid string) error {
	blkOut := ter.GetCmdOut("blkid | grep "+oldUuid+" | awk -F: '{print $1}'", time.Second*3)
	dev := ter.MatchStr(blkOut, `/dev/sd[a-z]{1}[0-9]{1,}`)
	//fmt.Println("[" + dev + "]")

	err := ter.RunCmd("mkfs.ext4 "+dev+" -F", time.Second*5)
	if err != nil {
		return err
	}
	return nil
}
