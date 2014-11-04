package main

import (
    "fmt"
)

func IsValidRoomName(name string) bool {
    channel := rds.Exists(fmt.Sprintf("channel:%v", name))
    if channel.Err() != nil {
	return false
    }
    return channel.Val()
}
