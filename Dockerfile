# Copyright 2020 ChainSafe Systems
# SPDX-License-Identifier: LGPL-3.0-only

FROM  golang:1.22 AS builder
ADD . /src
WORKDIR /src
RUN go mod download
RUN cd cmd/chainbridge && go build -o /bridge .

# # final stage
FROM debian:stable-slim
RUN apt-get -y update && apt-get -y upgrade && apt-get install ca-certificates -y

COPY --from=builder /bridge ./
RUN chmod +x ./bridge

ENTRYPOINT ["./bridge"]
