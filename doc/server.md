* 记得改timezone

* adduser henyouqian
* vim /etc/sudoers
** 增加 henyouqian ALL=(ALL) ALL
* sudo apt-get update
* sudo apt-get upgrade

- 配置ufw
sudo ufw enable
sudo ufw default deny 
sudo ufw allow 22
sudo ufw allow 80
sudo ufw allow 443
sudo ufw allow 9977

- 修改最大文件打开数
用 root 权限修改 /etc/sysctl.conf 文件:
fs.file-max = 1020000
net.ipv4.ip_conntrack_max = 1020000
net.ipv4.netfilter.ip_conntrack_max = 1020000

编辑 /etc/security/limits.conf 文件, 加入如下行:
# /etc/security/limits.conf
work         hard    nofile      1020000
work         soft    nofile      1020000
第一列的 work 表示 work 用户, 你可以填 *, 或者 root. 然后保存退出, 重新登录服务器.

- 安装redis
* sudo apt-get install redis-server

- 安装unzip

- 安装ssdb
wget --no-check-certificate https://github.com/ideawu/ssdb/archive/master.zip
unzip master
cd ssdb-master
make
sudo make install

- 安装git
sudo apt-get install git
git config --global credential.helper 'cache --timeout 36000'

- 安装tmux
sudo apt-get install tmux

- 安装go
sudo apt-get install golang

sudo vim ~/.profile
增加：
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

mkdir $HOME/go

- 安装go packege
go get github.com/henyouqian/lwutil
go get github.com/henyouqian/ssdbgo
go get github.com/henyouqian/fastimage
go get github.com/golang/glog
go get github.com/garyburd/redigo/redis
go get github.com/robfig/cron
go get github.com/qiniu/api/rs
go get github.com/gorilla/websocket

- 安装nginx
sudo apt-get install nginx

- ubuntu安全更新
sudo cp /etc/apt/sources.list /etc/apt/security.sources.list
sudo apt-get upgrade -o Dir::Etc::SourceList=/etc/apt/security.sources.list

- git clone https://github.com/henyouqian/SldServer.git

- 启动redis
redis-server redis.conf



