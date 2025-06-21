package main

import (
	"fmt"
	"testing"
)

func TestGetFileChunksTableDriven(t *testing.T) {
	type entry struct {
		file_path string
		
		expct_chunks []int
		expct_error bool 
	}

	var tests []entry = []entry{
		{
			file_path: "t_files/a",
			expct_chunks: []int{1024},
			expct_error: false,
		},
		{
			file_path: "t_files/b",
			expct_chunks: []int{1024, 512},
			expct_error: false,
		},
		{
			file_path: "t_files/dne",
			expct_chunks: nil,
			expct_error: true,
		},
	}

	for _, test := range tests {
		test_name := fmt.Sprintf("%s[%v]: %v", test.file_path, test.expct_error, test.expct_chunks)
		t.Run(test_name, func(t *testing.T) {
			chunks, err := get_file_chunks(test.file_path)
			if (err != nil) != test.expct_error {
				t.Fatalf("error-ed: got %v expected %v", err != nil, test.expct_error)
			}
			if len(chunks) != len(test.expct_chunks) {
				t.Fatalf("chunks_count: got %d expected %d", len(chunks), len(test.expct_chunks))
			}
			for i := range len(chunks) {
				if len(chunks[i]) != test.expct_chunks[i] {
					t.Fatalf("chunk_%d_length: got %d expected %d", i, len(chunks[i]), test.expct_chunks[i])
				}
			}
		})
	}
}
