package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/imroc/req"
	"github.com/urfave/cli/v2"
)

var noTearErr = errors.New("体力耗尽")
var noPayErr = errors.New("raca费用不足")

var (
	accessToken string
	fromAddress string
)

func main() {
	app := &cli.App{
		Name: "元兽游戏",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "address",
				Usage: "填写钱包地址",
			},
			&cli.StringFlag{
				Name:  "token",
				Usage: "请在login中粘贴你的token",
			},
		},
		Before: func(context *cli.Context) error {
			fromAddress = context.String("address")
			accessToken = context.String("token")
			return nil
		},
		Commands: []*cli.Command{
			{
				Name: "updatelevel",
				Action: func(c *cli.Context) error {
					if err := updateLevel(); err != nil {
						fmt.Println(err)
						return err
					}
					return nil
				},
			},
			{
				Name: "start",
				Action: func(c *cli.Context) error {
					fmt.Println("开始游戏")
					start()
					fmt.Println("游戏结束")
					return nil
				},
			},
			{
				Name: "mint",
				Action: func(c *cli.Context) error {
					if err := mint(); err != nil {
						fmt.Println(err)
						return err
					}
					return nil
				},
			},
			{
				Name: "checkbag",
				Action: func(c *cli.Context) error {
					racaCoin, pnum, err := checkBag()
					if err != nil {
						fmt.Println(err)
						return err
					}
					fmt.Println("余额:", racaCoin, "碎片", pnum)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		return
	}
}

func start() {
	ms, err := getAvailMetaMon()
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	c := make(chan Metamon, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for metamon := range c {
				fmt.Printf("metamon %d 开始战斗\n", metamon.ID)
				for {
					bid, err := getBattelObject(metamon.ID)
					if err != nil {
						fmt.Printf("metamon %d 获取对战对象失败\n", metamon)
						fmt.Println(err)
						break
					}
					err = battle(metamon.ID, bid)
					if err != nil {
						if err == noTearErr {
							fmt.Printf("metamon %d 没有体力\n", metamon.ID)
						} else {
							fmt.Printf("metamon %d 没有成功开始战斗\n", metamon.ID)
							fmt.Println(err)
						}
						break
					}
					racaCoin, pieceNum, err := checkBag()
					if err != nil {
						fmt.Println("获取背包失败,", err)
						break
					}
					fmt.Printf("获得碎片:%d\n", pieceNum)
					fmt.Printf("剩余raca余额:%d\n", racaCoin)
					if racaCoin < 50 {
						for {
							if racaCoin > 50 {
								fmt.Println("raca余额足够，战斗继续")
								break
							} else {
								fmt.Println("raca 余额不足，请充值")
							}
							racaCoin, _, err = checkBag()
							if err != nil {
								fmt.Println("获取背包失败,", err)
								break
							}
							time.Sleep(3 * time.Second)
						}
					}
				}
			}
		}()
	}

	fmt.Printf("当前有%d只元兽有体力\n", len(ms))
	for _, metamon := range ms {
		c <- metamon
	}
	close(c)
	wg.Wait()

	fmt.Println("战斗结束,开始mint")
	if err := mint(); err != nil {
		fmt.Println(err)
	}
	fmt.Println("mint 完成，开始升级")
	if err := updateLevel(); err != nil {
		fmt.Println(err)
	}
	fmt.Println("")
}

type Metamon struct {
	ID     int `json:"id"`
	Level  int `json:"level"`
	Exp    int `json:"exp"`
	ExpMax int `json:"expMax"`
	Tear   int `json:"tear"`
}

type GetAllMetaMonResult struct {
	Data struct {
		MetamonList []Metamon
	} `json:"data"`
}

type MetaMonProp struct {
	Data struct {
		Tear int `json:"tear"`
	} `json:"data"`
}

func getAvailMetaMon() ([]Metamon, error) {
	api := "https://metamon-api.radiocaca.com/usm-api/getWalletPropertyList"
	resp, err := req.Post(
		api, req.Param{"address": fromAddress, "pageSize": 300}, req.Header{"accesstoken": accessToken},
	)
	if err != nil {
		return nil, err
	}

	var rs GetAllMetaMonResult
	if resp.Response().StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("resp.Response().StatusCode:%d", resp.Response().StatusCode))
	}
	err = resp.ToJSON(&rs)
	if err != nil {
		return nil, err
	}

	var metamons []Metamon

	for _, meta := range rs.Data.MetamonList {
		if meta.Tear > 0 {
			metamons = append(metamons, meta)
		}
	}

	return metamons, nil
}

type BatterObjResult struct {
	Data struct {
		Objects []struct {
			ID int `json:"id"`
		}
	}
}

func getBattelObject(metaID int) (int, error) {
	api := "https://metamon-api.radiocaca.com/usm-api/getBattelObjects"
	resp, err := req.Post(
		api,
		req.Param{
			"address":   fromAddress,
			"metamonId": metaID,
			"front":     1,
		},
		req.Header{"accesstoken": accessToken},
	)
	if err != nil {
		return 0, err
	}
	var objs BatterObjResult
	err = resp.ToJSON(&objs)
	if err != nil {
		return 0, err
	}

	var ids []int
	for _, object := range objs.Data.Objects {
		ids = append(ids, object.ID)
	}
	if len(ids) > 0 {
		r := rand.Int31n(int32(len(ids)))
		return ids[int(r)], nil
	}
	return 0, err
}

func battle(metaIDA, metaIDB int) error {
	api := "https://metamon-api.radiocaca.com/usm-api/startBattle"
	resp, err := req.Post(
		api, req.Param{
			"address":     fromAddress,
			"monsterA":    metaIDA,
			"monsterB":    metaIDB,
			"battleLevel": 1,
		},
		req.Header{"accesstoken": accessToken},
	)
	if err != nil {
		return err
	}

	m := make(map[string]interface{})
	err = resp.ToJSON(&m)
	if err != nil {
		return err
	}
	ors := m["result"].(float64)
	if ors == 1 {
		return nil
	}
	msg := m["message"].(string)
	if strings.Contains(msg, "You didn't pay for the game") {
		return noPayErr
	}
	return noTearErr
}

type BagItem struct {
	Num int `json:"bpNum"`
	Typ int `json:"bpType"`
}

type Bag struct {
	Data struct {
		Items []BagItem `json:"item"`
	} `json:"data"`
}

func checkBag() (int, int, error) {
	api := "https://metamon-api.radiocaca.com/usm-api/checkBag"
	resp, err := req.Post(
		api, req.Param{
			"address": fromAddress,
		}, req.Header{"accesstoken": accessToken},
	)
	if err != nil {
		return 0, 0, err
	}
	bag := new(Bag)
	if err := resp.ToJSON(&bag); err != nil {
		return 0, 0, err
	}
	var (
		pieceNum int
		racaCoin int
	)
	for _, item := range bag.Data.Items {
		if item.Typ == 1 {
			pieceNum = item.Num
		}
		if item.Typ == 5 {
			racaCoin = item.Num
		}
	}
	return racaCoin, pieceNum, nil
}

func updateLevel() error {
	api := "https://metamon-api.radiocaca.com/usm-api/getWalletPropertyList"
	resp, err := req.Post(api, req.Param{"address": fromAddress}, req.Header{"accesstoken": accessToken})
	if err != nil {
		return err
	}

	var rs GetAllMetaMonResult
	if resp.Response().StatusCode != 200 {
		return errors.New(fmt.Sprintf("resp.Response().StatusCode:%d", resp.Response().StatusCode))
	}
	err = resp.ToJSON(&rs)
	if err != nil {
		return err
	}
	hasUpdateLevel := false
	for _, metamon := range rs.Data.MetamonList {
		if metamon.Exp == metamon.ExpMax {
			hasUpdateLevel = true
			updateApi := "https://metamon-api.radiocaca.com/usm-api/updateMonster"
			resp, err := req.Post(
				updateApi, req.Param{
					"address": fromAddress,
					"nftId":   metamon.ID,
				}, req.Header{"accesstoken": accessToken},
			)
			if err != nil {
				return err
			}
			if resp.Response().StatusCode == 200 {
				fmt.Printf("metamon %d 升级\n", metamon.ID)
			}
		}
	}
	if !hasUpdateLevel {
		return errors.New("目前没有任何需要升级的元兽")
	}
	return nil
}

func mint() (err error) {
	for {
		api := "https://metamon-api.radiocaca.com/usm-api/composeMonsterEgg"
		resp, err := req.Post(api, req.Param{"address": fromAddress}, req.Header{"accesstoken": accessToken})
		if err != nil {
			return err
		}

		result := make(map[string]interface{})
		if err = resp.ToJSON(&result); err != nil {
			return err
		}
		code := result["code"].(string)
		if code == "SUCCESS" {
			_, num, err := checkBag()
			if err != nil {
				return err
			}
			fmt.Printf("合蛋成功，剩余碎片:%d\n", num)
			continue
		}

		return errors.New("没有足够碎片合成元兽蛋")
	}
}
