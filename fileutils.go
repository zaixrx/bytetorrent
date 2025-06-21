package main

import (
	"io"
	"os"
)

type Chunk []byte

func get_file_chunks(file_path string) ([]Chunk, error) {
	fs, err := os.Open(file_path)
	if err != nil {
		return nil, err
	}

	chunks := make([]Chunk, 0)
	buff := make([]byte, DataChunkSize)

	for {
		nbr, err := fs.Read(buff)
		if nbr == 0 && err == io.EOF {
			return chunks, nil		
		} else if err != nil {
			return nil, err
		}
		chunks = append(chunks, Chunk(buff[:nbr]))
	}
}
