# Alpine is chosen for anyflashcardsbot's image
FROM golang:alpine

# Create directory in image, it will be default for all commands
WORKDIR /anyflashcardsbot

# Copy configs
COPY configs ./configs

# Copy module files in to the image
COPY cmd/anyflashcardsbot ./
##COPY ./cmd/go.sum ./

# Execute command in image for downloading modules
##RUN go mod download

# Copy all *.go files
##COPY ./cmd/*.go ./

# Compile anyflashcardsbot in image
RUN go build -o /anyflashcardsbot

# Run anyflashcardsbot in image
CMD [ "./anyflashcardsbot" ]