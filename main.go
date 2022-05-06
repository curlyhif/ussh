package main

import (
	"cmdexec/ussh"
	"fmt"
	"reflect"
)

func main() {
	ussh.InitLog()
	sHand, err := ussh.InitTerminal("159.75.123.7:22", "ubuntu", "dx3906@dx")
	if err != nil {
		fmt.Println("ERROR:", err)
	}
	fmt.Println("############:su执行")
	err = sHand.GetSudoRoot("dx3906@dx")
	if err != nil {
		fmt.Println("ERROR0:", err)
		return
	}
	sHand.CloseHander()
}

func DumpMyName() {
	type em struct{}
	fmt.Println(reflect.TypeOf(em{}).PkgPath())
}
