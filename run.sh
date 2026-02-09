#!/bin/bash

docker run --name book-server-2 --env-file .env -v "$(pwd)/data:/data" spacecoupe/book_serverv2
