package main

import (
	pb "github.com/yeongcheon/pero-chat/gen/go"
)

type Room struct {
	Id       string
	RommInfo *pb.Room
	Users    map[string]*User
	Streams  []chan *pb.ChatMessageResponse
}
