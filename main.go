package main

import (
	"context"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
	"github.com/google/uuid"
	pb "github.com/yeongcheon/pero-chat/gen/go"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"net"
	"time"
)

type PeroChat struct {
	FirebaseAuthClient *auth.Client
	Rooms              map[string]*Room // key: room ID
}

func (p *PeroChat) Broadcast(ctx context.Context, messageRequest *pb.ChatMessageRequest) (*pb.BroadcastResponse, error) {
	roomId := messageRequest.GetRoomId()
	room := p.Rooms[roomId]
	if room == nil {
		return nil, errRoomNotExist
	}

	if room.Users == nil {
		room.Users = make(map[string]*User)
	}

	uid := ctx.Value("uid").(string)

	if _, ok := room.Users[uid]; !ok {
		return nil, errUserNotInRoom
	}

	u := room.Users[uid]
	user := &pb.User{
		Id:   uid,
		Name: u.Name,
		CreatedAt: &timestamppb.Timestamp{
			Seconds: int64(u.CreatedAt.Second()),
		},
	}

	message := &pb.ChatMessageResponse{
		MessageType: pb.ChatMessageResponse_COMMON_MESSAGE,
		Payload: &pb.ChatMessageResponse_CommonMessage{
			CommonMessage: &pb.CommonMessage{
				Id:      uuid.New().String(),
				User:    user,
				Message: messageRequest.GetMessage(),
				CreatedAt: &timestamppb.Timestamp{
					Seconds: time.Now().Unix() / 1000,
				},
			},
		},
	}

	for _, channel := range room.Streams {
		channel <- message
	}

	return &pb.BroadcastResponse{
		Message: "success",
	}, nil
}

func (p *PeroChat) Entry(entryRequest *pb.EntryRequest, stream pb.ChatService_EntryServer) error {
	if p.Rooms == nil {
		p.Rooms = make(map[string]*Room)
	}

	roomId := entryRequest.GetRoomId()
	if p.Rooms[roomId] == nil {
		p.Rooms[roomId] = &Room{
			Id: roomId,
		}
	}
	room := p.Rooms[roomId]

	ctx := stream.Context()

	uid := ctx.Value("uid").(string)

	record, err := p.FirebaseAuthClient.GetUser(ctx, uid)
	if err != nil {
		return errInvalidUserId
	}

	if room.Users == nil {
		room.Users = make(map[string]*User)
	}

	room.Users[uid] = &User{
		Id:        uid,
		Name:      record.DisplayName,
		CreatedAt: time.Unix(record.UserMetadata.CreationTimestamp, 0),
	}

	myChannel := make(chan *pb.ChatMessageResponse)
	room.Streams = append(p.Rooms[roomId].Streams, myChannel)

	for {
		message := <-myChannel

		err := stream.Send(message)
		if err != nil {
			for idx, ch := range room.Streams { // remove channel
				if ch == myChannel {
					room.Streams = append(room.Streams[:idx], room.Streams[idx+1:]...)
				}
			}

			ctx := stream.Context()
			uid := ctx.Value("uid").(string)

			delete(room.Users, uid)

			return err
		}
	}
}

func main() {
	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatalf("failed to listen : %v", err)
	}

	opts := []grpc.ServerOption{
		// grpc.Creds(creds),
		grpc.StreamInterceptor(firebaseAuthStreamInterceptor()),
		grpc.ChainUnaryInterceptor(
			unaryLoggingInterceptor,
			firebaseAuthInterceptor(),
		),
	}

	firebaseOpt := option.WithCredentialsFile("./firebase-adminsdk.json")
	app, err := firebase.NewApp(context.Background(), nil, firebaseOpt)
	if err != nil {
		log.Fatalf("error firebase init app: %v\n", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("error firebase auth client: %v\n", err)
	}

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterChatServiceServer(grpcServer, &PeroChat{
		FirebaseAuthClient: client,
	})
	if err := grpcServer.Serve(lis); err != nil {
		panic(err)
	}
}
