package main

import (
	"context"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
	"github.com/google/uuid"
	pb "github.com/yeongcheon/pero-chat/gen/go"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"net"
	"time"
)

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
)

type PeroChat struct {
	FirebaseAuthClient *auth.Client
	Rooms              map[string]*Room // key: room ID
}

func (p *PeroChat) Broadcast(ctx context.Context, messageRequest *pb.ChatMessageRequest) (*pb.BroadcastResponse, error) {
	roomId := messageRequest.GetRoomId()
	room := p.Rooms[roomId]
	if room == nil {
		return nil, grpc.Errorf(codes.NotFound, "room %s is not exist", roomId)
	}

	if room.Users == nil {
		room.Users = make(map[string]*User)
	}

	uid := ctx.Value("uid").(string)

	if _, ok := room.Users[uid]; !ok {
		return nil, errInvalidToken // FIXME
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
		return errInvalidToken // FIXME
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

func firebaseAuthStreamInterceptor() grpc.StreamServerInterceptor {
	opt := option.WithCredentialsFile("./firebase-adminsdk.json")
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("error firebase init app: %v\n", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("firebase auth client init err: %v\n", err)
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if len(md["authorization"]) >= 1 {
				idToken := md["authorization"][0]

				authToken, err := client.VerifyIDToken(ctx, idToken)
				if err != nil {
					return err
				}

				ctx = context.WithValue(ctx, "uid", authToken.UID)
			}
		} else {
			return errInvalidToken
		}

		return handler(srv, newWrappedStream(ctx, ss))
	}
}

func firebaseAuthInterceptor() grpc.UnaryServerInterceptor {
	opt := option.WithCredentialsFile("./firebase-adminsdk.json")
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("error firebase init app: %v\n", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("firebase auth client init err: %v\n", err)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			authorization := md["authorization"]
			if len(authorization) == 0 {
				return nil, errInvalidToken
			}

			idToken := authorization[0]

			authToken, err := client.VerifyIDToken(ctx, idToken)
			if err != nil {
				return nil, err
			}

			ctx = context.WithValue(ctx, "uid", authToken.UID)
			return handler(ctx, req)
		} else {
			return nil, errInvalidToken
		}
	}
}

func unaryLoggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Printf("%s", req)
	return handler(ctx, req)
}

func main() {
	_, _ = credentials.NewServerTLSFromFile("ssl.crt", "ssl.key")
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
