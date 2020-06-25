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

type Room struct {
	Id       string
	RommInfo *pb.Room
	Users    []*User
	Streams  []pb.ChatService_EntryServer
}

type User struct {
}

type PeroChat struct {
	FirebaseAuthClient *auth.Client
	Rooms              map[string][]chan *pb.ChatMessageResponse // key: room ID
}

func (p *PeroChat) Broadcast(ctx context.Context, messageRequest *pb.ChatMessageRequest) (*pb.BroadcastResponse, error) {
	roomId := messageRequest.GetRoomId()
	room := p.Rooms[roomId]
	if room == nil {
		return nil, grpc.Errorf(codes.NotFound, "room %s is not exist", roomId)
	}

	uid := ctx.Value("uid").(string)
	sec := time.Now().Unix() / 1000

	record, err := p.FirebaseAuthClient.GetUser(ctx, uid)
	if err != nil {
		return nil, errInvalidToken
	}

	user := &pb.User{
		Id:   uid,
		Name: record.DisplayName,
		CreatedAt: &timestamppb.Timestamp{
			Seconds: record.UserMetadata.CreationTimestamp / 1000,
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
					Seconds: sec,
				},
			},
		},
	}

	for _, channel := range room {
		channel <- message
	}

	return &pb.BroadcastResponse{
		Message: "success",
	}, nil
}

func (p *PeroChat) Entry(entryRequest *pb.EntryRequest, stream pb.ChatService_EntryServer) error {
	if p.Rooms == nil {
		p.Rooms = make(map[string][]chan *pb.ChatMessageResponse)
	}

	roomId := entryRequest.GetRoomId()
	if p.Rooms[roomId] == nil {
		p.Rooms[roomId] = make([]chan *pb.ChatMessageResponse, 0)
	}
	myChannel := make(chan *pb.ChatMessageResponse)
	p.Rooms[roomId] = append(p.Rooms[roomId], myChannel)

	for {
		message := <-myChannel

		err := stream.Send(message)
		if err != nil {
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

				_, err := client.VerifyIDToken(ctx, idToken)
				if err != nil {
					return err
				}
			}
		} else {
			return errInvalidToken
		}

		return handler(srv, ss)
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
	lis, err := net.Listen("tcp", ":9999")
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
