# 介绍

raca 元兽游戏辅助脚本

# 注意
raca api有时候不太稳定，如果遇到程序中断或者start之后依然还有元兽有体力，可以多手动执行几次start确保已经全部打完

# 下载
请在bin目录下载对应操作系统的二进制文件

# 使用

## 开始游戏, 完成游戏会自动合成元兽蛋并升级元兽（如果有碎片和药水），一般用这个就可以了

### linux/mac

./metamon --address={钱包地址} --token={accesstoken} start

### windows

./metamon.exe --address={钱包地址} --token={accesstoken} start

## 合成元兽蛋

### linux/mac

./metamon --address={钱包地址} --token={accesstoken} mint

### windows

./metamon.exe --address={钱包地址} --token={accesstoken} mint

## 升级可升级的元兽

### linux/mac

./metamon --address={钱包地址} --token={accesstoken} updatelevel

### windows

./metamon.exe --address={钱包地址} --token={accesstoken} updatelevel
