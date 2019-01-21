// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package dashboard

import "time"

//DefaultConfig包含仪表板的默认设置。
// DefaultConfig contains default settings for the dashboard.
var DefaultConfig = Config{
	Host:    "localhost",
	Port:    8080,
	Refresh: 5 * time.Second,
}

// Config contains the configuration parameters of the dashboard./Config包含仪表板的配置参数。
type Config struct {
	//host是启动仪表板服务器的主机接口。如果此/字段为空，则不会启动仪表板。
	// Host is the host interface on which to start the dashboard server. If this
	// field is empty, no dashboard will be started.
	Host string `toml:",omitempty"`
	//Port是启动仪表板服务器的TCP端口号。/默认值为/有效，将随机选择一个端口号(有用/用于临时节点)。
	// Port is the TCP port number on which to start the dashboard server. The
	// default zero value is/ valid and will pick a port number randomly (useful
	// for ephemeral nodes).
	Port int `toml:",omitempty"`
	//Refresh是数据更新的刷新速率，将经常收集chartEntry。
	// Refresh is the refresh rate of the data updates, the chartEntry will be collected this often.
	Refresh time.Duration `toml:",omitempty"`
}
