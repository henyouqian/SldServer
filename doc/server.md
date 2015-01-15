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
go get github.com/golang/glog
go get github.com/garyburd/redigo/redis
go get github.com/robfig/cron
go get github.com/qiniu/api/rs
go get github.com/gorilla/websocket
go get github.com/rubenfonseca/fastimage

- 安装nginx
sudo apt-get install nginx

- ubuntu安全更新
sudo cp /etc/apt/sources.list /etc/apt/security.sources.list
sudo apt-get upgrade -o Dir::Etc::SourceList=/etc/apt/security.sources.list

- git clone https://github.com/henyouqian/SldServer.git

- 启动redis
redis-server redis.conf



