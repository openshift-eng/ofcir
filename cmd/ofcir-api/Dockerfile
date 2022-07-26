FROM golang:1.18-alpine
RUN mkdir /ofcir
ADD . /ofcir
WORKDIR /ofcir
RUN go build -o api .
CMD ["/ofcir/api"]
