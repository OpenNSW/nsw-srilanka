module github.com/OpenNSW/nsw-srilanka

go 1.27

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/LSFLK/argus/pkg/audit v0.0.0-20260616134204-858f4ee608e8
	github.com/OpenNSW/core v0.0.0-20260824093457-2f2a10476dda
	github.com/OpenNSW/core/authn v0.3.0
	github.com/OpenNSW/core/authz v0.1.0
	github.com/OpenNSW/core/database v0.1.0
	github.com/OpenNSW/core/httputil v0.1.0
	github.com/OpenNSW/core/pagination v0.1.0
	github.com/OpenNSW/core/payment v0.2.1-0.20260827022843-b3d362eee13a
	github.com/OpenNSW/core/remote v0.8.0
	github.com/OpenNSW/core/shared v0.3.1-0.20260827022843-b3d362eee13a
	github.com/OpenNSW/core/storage v0.1.1-0.20260827022843-b3d362eee13a
	github.com/OpenNSW/core/trace v0.2.0
	github.com/OpenNSW/core/uiprojector v0.3.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/shopspring/decimal v1.4.0
	github.com/stretchr/testify v1.12.1
	go.temporal.io/sdk v1.48.0
	gorm.io/driver/postgres v1.6.2
	gorm.io/gorm v1.31.2
)

require (
	github.com/OpenNSW/core/secret v0.2.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.43.7 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.18 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.38 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.37 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.39 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.39 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.107.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.7 // indirect
	github.com/aws/smithy-go v1.27.8 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/nexus-rpc/nexus-proto-annotations v0.1.0 // indirect
	github.com/nexus-rpc/sdk-go v0.7.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	go.temporal.io/api v1.63.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.39.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260504160031-60b97b32f348 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260504160031-60b97b32f348 // indirect
	google.golang.org/grpc v1.82.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/OpenNSW/core/authn => github.com/OpenNSW/core/authn v0.2.0

replace github.com/OpenNSW/core/payment => github.com/OpenNSW/core/payment v0.2.1-0.20260827022843-b3d362eee13a

replace github.com/OpenNSW/core/shared => github.com/OpenNSW/core/shared v0.3.1-0.20260827022843-b3d362eee13a

replace github.com/OpenNSW/core/storage => github.com/OpenNSW/core/storage v0.1.1-0.20260827022843-b3d362eee13a
