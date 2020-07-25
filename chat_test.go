package main

import (
	"context"
	firebase "firebase.google.com/go"
	"github.com/stretchr/testify/assert"
	pb "github.com/yeongcheon/pero-chat/gen/go"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"log"
	"net"
	"testing"
)

const (
	serverAddr = "localhost:9090"
)

func TestPeroChat(t *testing.T) {
	initServer(t)

	conn, err := grpc.Dial(serverAddr,
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Error(err)
	}

	roomId := "rommId"

	c := pb.NewChatServiceClient(conn)
	entryClient, entryErr := c.Entry(context.Background(), &pb.EntryRequest{
		RoomId: roomId,
	})
	if entryErr != nil {
		t.Error(err)
	}
	defer entryClient.CloseSend()

	res, err := c.Broadcast(context.Background(), &pb.ChatMessageRequest{
		RoomId:  roomId,
		Message: "message",
	})
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, roomId, res.GetRoom().GetId(), "The two rooms should be the same.")
}

func initServer(t *testing.T) {
	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatalf("failed to listen : %v", err)
	}

	firebaseOpt := option.WithCredentialsFile("./firebase-adminsdk.json")
	app, err := firebase.NewApp(context.Background(), nil, firebaseOpt)
	if err != nil {
		t.Error(err)
	}

	client, err := app.Auth(context.Background())

	opts := []grpc.ServerOption{
		grpc.StreamInterceptor(firebaseAuthStreamInterceptor()),
		grpc.ChainUnaryInterceptor(
			unaryLoggingInterceptor,
			firebaseAuthInterceptor(),
		),
	}

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterChatServiceServer(grpcServer, &PeroChat{
		FirebaseAuthClient: client,
	})

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			panic(err)
		}
	}()
}
