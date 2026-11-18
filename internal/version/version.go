// Package version 集中维护全项目版本号。
// Hub 与 Agent 是同一二进制，版本必须一致比较，因此放在共享包。
package version

// Version 为构建时通过 -ldflags "-X github.com/SinGyoZ/TarShip/internal/version.Version=vX.Y.Z"
// 注入的版本号；直接 go build 时使用默认值。
var Version = "0.1.0-dev"
