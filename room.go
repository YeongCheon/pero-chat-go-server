package main

import (
	pb "github.com/yeongcheon/pero-chat/gen/go"
)

type Room struct {
	Id       string
	RoomInfo *pb.Room
	Users    map[string]*User
	Streams  []chan *pb.ChatMessageResponse
}
