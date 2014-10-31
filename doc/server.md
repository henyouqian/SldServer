* 记得改timezone

* adduser henyouqian
* vim /etc/sudoers
** 增加 your_user_name ALL=(ALL) ALL
* sudo apt-get update
* sudo apt-get upgrade

- 安装redis
* sudo apt-get install redis

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

- git clone https://github.com/henyouqian/SldServer.git

- 安装go packege
go get github.com/henyouqian/lwutil
go get github.com/golang/glog
go get github.com/garyburd/redigo/redis
go get github.com/robfig/cron
go get github.com/qiniu/api/rs

- 安装nginx
sudo apt-get install nginx