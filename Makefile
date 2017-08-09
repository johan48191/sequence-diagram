sequence-diagram: *.go
	go build -o $@ .

example.svg: example.txt sequence-diagram
	./sequence-diagram < example.txt > example.svg
