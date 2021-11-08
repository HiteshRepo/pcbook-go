package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"

	"github.com/google/uuid"
	"github.com/hiteshrepo/pcbook/pb"
	"google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

const maxImageSize = 1 << 20 // 1 MB

type LaptopServer struct {
	pb.UnimplementedLaptopServiceServer
	LaptopStore LaptopStore
	ImageStore  ImageStore
	RatingStore RatingStore
}

func NewLaptopServer(laptopStore LaptopStore, imageStore ImageStore, ratingStore RatingStore) *LaptopServer {
	return &LaptopServer{LaptopStore: laptopStore, ImageStore: imageStore, RatingStore: ratingStore}
}

// CreateLaptop is a unary RPC that creates a new laptop
func (server *LaptopServer) CreateLaptop(ctx context.Context, req *pb.CreateLaptopRequest) (*pb.CreateLaptopResponse, error) {
	laptop := req.GetLaptop()
	log.Printf("receive a create-laptop request with id: %s", laptop.Id)

	if len(laptop.Id) > 0 {
		_, err := uuid.Parse(laptop.Id)

		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "laptop id is not a valid UUID: %v", err)
		}
	} else {
		id, err := uuid.NewRandom()

		if err != nil {
			return nil, status.Errorf(codes.Internal, "laptop id could not be genrated: %v", err)
		}

		laptop.Id = id.String()
	}

	if err := contextError(ctx); err != nil {
		return nil, err
	}

	// save laptop to in-memory storage
	err := server.LaptopStore.Save(laptop)
	if err != nil {
		code := codes.Internal
		if errors.Is(err, ErrAlreadyExists) {
			code = codes.AlreadyExists
		}
		return nil, status.Errorf(code, "cannot save laptop to the store: %v", err)
	}

	log.Printf("laptop created with id: %s", laptop.Id)
	return &pb.CreateLaptopResponse{Id: laptop.Id}, nil
}

// SearchLaptop is a server streaming rpc service to search for laptops
func (server *LaptopServer) SearchLaptop(req *pb.SearchLaptopRequest, stream pb.LaptopService_SearchLaptopServer) error {
	filter := req.GetFilter()
	log.Printf("receive a search-laptop request with filter: %v", filter)

	err := server.LaptopStore.Search(
		stream.Context(),
		filter,
		func(laptop *pb.Laptop) error {
			res := &pb.SearchLaptopResponse{Laptop: laptop}

			err := stream.Send(res)
			if err != nil {
				return err
			}

			log.Printf("sent laptop with id: %s", laptop.GetId())
			return nil
		},
	)

	if err != nil {
		return status.Errorf(codes.Internal, "unexpected error: %v", err)
	}

	return nil
}

// UploadImage is a client streaming rpc to upload image
func (server *LaptopServer) UploadImage(stream pb.LaptopService_UploadImageServer) error {

	req, err := stream.Recv()
	if err != nil {
		return logError(status.Errorf(codes.Unknown, "cannot receive image"))
	}

	laptopID := req.GetInfo().GetLaptopId()
	imageType := req.GetInfo().GetImageType()
	log.Printf("receive an upload image request for laptop %s of image type %s", laptopID, imageType)

	laptop, err := server.LaptopStore.Find(laptopID)
	if err != nil {
		return logError(status.Errorf(codes.Internal, "cannot find laptop: %v", err))
	}

	if laptop == nil {
		return logError(status.Errorf(codes.NotFound, "cannot find laptop with id: %s", laptopID))
	}

	imageData := bytes.Buffer{}
	imageSize := 0

	for {

		if err := contextError(stream.Context()); err != nil {
			return err
		}

		log.Print("waiting for chunk data")

		req, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return logError(status.Errorf(codes.Unknown, "cannot receive chunk image data: %v", err))
		}

		chunk := req.GetChunkData()
		size := len(chunk)

		log.Printf("received image chunk of size: %d", size)

		imageSize += size
		if imageSize > maxImageSize {
			return logError(status.Errorf(codes.InvalidArgument, "image is too large: %d > %d", imageSize, maxImageSize))
		}

		_, err = imageData.Write(chunk)
		if err != nil {
			return logError(status.Errorf(codes.Internal, "cannot write image chunk data to file: %v", err))
		}
	}

	imageID, err := server.ImageStore.Save(laptopID, imageType, imageData)
	if err != nil {
		return logError(status.Errorf(codes.Internal, "cannot save image: %v", err))
	}

	res := &pb.UploadImageResponse{
		Id:   imageID,
		Size: uint32(imageSize),
	}

	err = stream.SendAndClose(res)
	if err != nil {
		return logError(status.Errorf(codes.Unknown, "cannot send response: %v", err))
	}

	log.Printf("saved image to store with id: %s and size: %d", imageID, imageSize)
	return nil
}

func logError(err error) error {
	if err != nil {
		log.Print(err)
	}
	return err
}

func contextError(ctx context.Context) error {
	if ctx.Err() == context.Canceled {
		log.Print("request is cancelled")
		return status.Error(codes.Canceled, "request is cancelled")
	}

	if ctx.Err() == context.DeadlineExceeded {
		log.Print("deadline exceeded")
		return status.Error(codes.DeadlineExceeded, "deadline is exceeded")
	}

	return nil
}

// RateLaptop is a bidirectional-streaming RPC that allows client to rate a stream of laptops
// with a score, and returns a stream of average score for each of them
func (server *LaptopServer) RateLaptop(stream pb.LaptopService_RateLaptopServer) error {
	for {
		err := contextError(stream.Context())
		if err != nil {
			return err
		}

		req, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return logError(status.Errorf(codes.Unknown, "cannot receive stream request: %v", err))
		}

		laptopID := req.GetLaptopId()
		score := req.GetScore()

		log.Printf("received  a rate-alptop request: id = %s, score = %0.2f", laptopID, score)

		found, err := server.LaptopStore.Find(laptopID)
		if err != nil {
			return logError(status.Errorf(codes.Internal, "cannot find laptop: %v", err))
		}

		if found == nil {
			return logError(status.Errorf(codes.NotFound, "no laptop found with id: %s", laptopID))
		}

		rating, err := server.RatingStore.Add(laptopID, score)
		if err != nil {
			return logError(status.Errorf(codes.Internal, "could not add rating: %v", err))
		}

		res := &pb.RateLaptopResponse{
			LaptopId:     laptopID,
			RatedCount:   rating.Count,
			AverageScore: rating.Sum / float64(rating.Count),
		}

		err = stream.Send(res)
		if err != nil {
			return logError(status.Errorf(codes.Unknown, "could not send response: %v", err))
		}

	}
	return nil
}
