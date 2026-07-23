# 从 proto 生成可在浏览器直接使用的 TypeScript 客户端，输出到前端目录
# Generates a web-safe TypeScript client from a proto, into the frontend

install:
	npm install -g @protobuf-ts/plugin
	go install github.com/yylego/kratos-ts-http/cmd/protoc-gen-kratos-ts-http@latest

generate:
	buf generate --template buf.gen.ts.yaml --include-imports -o web/src/rpc

# 比较生成的结果是否符合预期
# Check if the generated result matches the expected one
check:
	diff ./web/src/rpc/greeter/v1/greeter.client.ts ./golden.ts

.PHONY: install generate check
