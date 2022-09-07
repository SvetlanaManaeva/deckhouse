FROM alpine:3.14
RUN apk add --no-cache make go curl bash
COPY Makefile /deckhouse/Makefile
COPY tools/regcopy /deckhouse/tools/regcopy
WORKDIR /deckhouse
