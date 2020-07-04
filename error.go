package main

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errUserNotInRoom   = status.Errorf(codes.InvalidArgument, "user not eixst in room")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
	errInvalidUserId   = status.Errorf(codes.InvalidArgument, "invalid user id")
	errRoomNotExist    = status.Errorf(codes.NotFound, "room is not exist")
)
