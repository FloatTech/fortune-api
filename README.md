# fortune-api
基于图片缓存池的，以tx为图床的运势API

## 部署
```bash
go run main.go 0.0.0.0:8000
```
或
```bash
go build -ldflags "-s -w" -trimpath -o fortune
./fortune 0.0.0.0:8000
```

## API
> http://127.0.0.1:8000/fortune?id=123456&kind=车万

- **id**: 抽运势的人的唯一标识，必填
- **kind**: 运势底图类型，不填默认`车万`，详见 [ZeroBot-Plugin](https://github.com/FloatTech/ZeroBot-Plugin)