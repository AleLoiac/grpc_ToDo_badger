package main

import (
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"grpc_ToDo_badger/todopb"
	"log"
	"net"
	"strconv"
	"time"
)

type server struct {
	todopb.ToDoServiceServer
}

var todoList []*todopb.ToDo

var serial int

func (*server) CreateToDo(ctx context.Context, req *todopb.NewToDo) (*todopb.ToDoResponse, error) {
	fmt.Printf("Create function is invoked with %v\n", req)
	id := strconv.Itoa(serial)
	title := req.GetTitle()
	description := req.GetDescription()

	serial++

	newTodo := &todopb.ToDo{
		Id:          id,
		Title:       title,
		Description: description,
		Done:        false,
	}

	res := &todopb.ToDoResponse{
		Todo: newTodo,
	}

	err := db.Update(func(txn *badger.Txn) error {
		data, err := proto.Marshal(newTodo)
		if err != nil {
			return err
		}
		return txn.Set([]byte(id), data)
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return res, nil
}

func (*server) ListToDos(req *todopb.Empty, stream todopb.ToDoService_ListToDosServer) error {
	fmt.Println("ListToDos function is invoked with an empty request")

	resTodo := &todopb.ToDo{}

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			data := it.Item()
			err := data.Value(func(v []byte) error {
				err := proto.Unmarshal(v, resTodo)
				res := &todopb.ToDoResponse{
					Todo: &todopb.ToDo{
						Id:          resTodo.GetId(),
						Title:       resTodo.GetTitle(),
						Description: resTodo.GetDescription(),
						Done:        resTodo.GetDone(),
					},
				}
				err = stream.Send(res)
				if err != nil {
					return err
				}
				time.Sleep(500 * time.Millisecond)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil

}

func (*server) CheckUncheck(ctx context.Context, req *todopb.ToDoId) (*todopb.ToDoResponse, error) {
	fmt.Printf("CheckUncheck function is invoked with %v\n", req)
	id := req.GetId()

	var resTodo *todopb.ToDoResponse
	err := db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(id))
		if err != nil {
			return status.Errorf(codes.NotFound, fmt.Sprintf("Cannot find todo with id: %v", id))
		}

		var controlTodo todopb.ToDo
		err = item.Value(func(val []byte) error {
			err = proto.Unmarshal(val, &controlTodo)
			if err != nil {
				return err
			}
			controlTodo.Done = !controlTodo.Done
			resTodo = &todopb.ToDoResponse{Todo: &controlTodo}
			newVal, err := proto.Marshal(&controlTodo)
			if err != nil {
				return err
			}
			return txn.SetEntry(&badger.Entry{
				Key:   []byte(id),
				Value: newVal,
			})
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resTodo, nil
}

func (*server) DeleteToDo(ctx context.Context, req *todopb.ToDoId) (*todopb.Empty, error) {
	fmt.Printf("DeleteToDo function is invoked with %v\n", req)
	id := req.GetId()

	for i := range todoList {
		if id == todoList[i].GetId() {
			todoList = append(todoList[:i], todoList[i+1:]...)
			res := &todopb.Empty{}
			fmt.Println("Todo deleted")
			return res, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Cannot find todo with id: %v", req.GetId()))
}

var db *badger.DB

func main() {
	fmt.Println("Server started...")

	db, _ = badger.Open(badger.DefaultOptions("/Users/aless/Desktop/Go/grpc_ToDo_badger/db"))

	defer db.Close()

	//todoList = make([]*todopb.ToDo, 0)

	lis, err := net.Listen("tcp", "0.0.0.0:50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	todopb.RegisterToDoServiceServer(s, &server{})

	reflection.Register(s)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
