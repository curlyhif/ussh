package main

import (
	"cmdexec/Test"
	"cmdexec/ussh"
	"fmt"
)

func main() {
	ussh.InitLog(ussh.LOG_IO_LOGFILE)
	var tran Test.TranSto
	tran.Port = "22"
	tran.TPort = "22"
	tran.IP = "10.0.2.113"
	tran.TIp = "10.0.2.93"
	tran.Passwd = "yourpasswd"
	tran.User = "yourname"
	tran.NewUuid = "e11f72df-bc12-4084-b9dd-85795b9153aa"
	tran.OldUuid = "63b08e8f-1e1b-4739-b3f7-3aa64bdaee3e"
	err := Test.TranUuidsto(tran)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("success")
	}
}
