FROM golang:1.25-alpine
RUN mkdir /ofcir
ADD . /ofcir
WORKDIR /ofcir
RUN go build -o api .
CMD ["/ofcir/api"]
