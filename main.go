package main

import (
	"errors"
	"fmt"
	"github.com/imroc/req"
	"github.com/urfave/cli/v2"
	"go.uber.org/atomic"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
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

var wg sync.WaitGroup

func battleProcess(total, wins *atomic.Int32, metamon Metamon) {
	defer wg.Done()
	fmt.Printf("metamon %d 开始战斗\n", metamon.ID)
	for {
		bid, err := getBatteleObject(metamon.ID)
		if err != nil {
			fmt.Printf("metamon %d 获取对战对象失败\n", metamon)
			fmt.Println(err)
			continue
		}
		win, err := battle(metamon.ID, bid)
		if err != nil {
			if err == noTearErr {
				fmt.Printf("metamon %d 没有体力\n", metamon.ID)
				break
			}
			fmt.Printf("metamon %d 没有成功开始战斗,重试\n", metamon.ID)
			fmt.Println(err)
			continue
		}

		total.Add(1)
		if win {
			wins.Add(1)
		}

		racaCoin, _, err := checkBag()
		if err != nil {
			fmt.Println("获取背包失败,", err)
			continue
		}
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
					continue
				}
				time.Sleep(3 * time.Second)
			}
		}
		if err = updateLevelByID(metamon.ID); err != nil {
			fmt.Println(err)
			continue
		}
	}
}

func start() {
	total := atomic.NewInt32(int32(0))
	wins := atomic.NewInt32(int32(0))

	ms, err := getAvailMetaMon()
	if err != nil {
		panic(err)
	}

	_, cpum, err := checkBag()
	if err != nil {
		panic(err)
	}
	if len(ms) == 0 {
		fmt.Println("当前没有任何元兽有体力")
		return
	}

	fmt.Printf("当前有%d只元兽有体力\n", len(ms))
	for _, metamon := range ms {
		wg.Add(1)
		go battleProcess(total, wins, metamon)
	}
	wg.Wait()

	_, pnum, err := checkBag()
	fmt.Printf("战斗结束，当前碎片数量:%d，今天战斗获取数量:%d, 胜率:%.2f\n", pnum, pnum-cpum,
		float64(wins.Load())/float64(total.Load()))

	fmt.Println("战斗结束,开始mint")
	if err := mint(); err != nil {
		fmt.Println(err)
	}
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

func getBatteleObject(metaID int) (int, error) {
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

type BatterResult struct {
	Code string `json:"code"`
	Data struct {
		BattleLevel      int `json:"battleLevel"`
		BpFragmentNum    int `json:"bpFragmentNum"`
		BpPotionNum      int `json:"bpPotionNum"`
		ChallengeExp     int `json:"challengeExp"`
		ChallengeLevel   int `json:"challengeLevel"`
		ChallengeMonster struct {
			Con           int         `json:"con"`
			ConMax        int         `json:"conMax"`
			CreateTime    string      `json:"createTime"`
			Crg           int         `json:"crg"`
			CrgMax        int         `json:"crgMax"`
			Exp           int         `json:"exp"`
			ExpMax        int         `json:"expMax"`
			ID            int         `json:"id"`
			ImageURL      string      `json:"imageUrl"`
			Inte          int         `json:"inte"`
			InteMax       int         `json:"inteMax"`
			Inv           int         `json:"inv"`
			InvMax        int         `json:"invMax"`
			IsPlay        bool        `json:"isPlay"`
			ItemID        int         `json:"itemId"`
			ItemNum       int         `json:"itemNum"`
			LastOwner     string      `json:"lastOwner"`
			Level         int         `json:"level"`
			LevelMax      int         `json:"levelMax"`
			Life          int         `json:"life"`
			LifeLL        int         `json:"lifeLL"`
			Luk           int         `json:"luk"`
			LukMax        int         `json:"lukMax"`
			MonsterUpdate bool        `json:"monsterUpdate"`
			Owner         string      `json:"owner"`
			Race          string      `json:"race"`
			Rarity        string      `json:"rarity"`
			Sca           int         `json:"sca"`
			ScaMax        int         `json:"scaMax"`
			Status        int         `json:"status"`
			Tear          int         `json:"tear"`
			TokenID       interface{} `json:"tokenId"`
			UpdateTime    string      `json:"updateTime"`
			Years         int         `json:"years"`
		} `json:"challengeMonster"`
		ChallengeMonsterID int `json:"challengeMonsterId"`
		ChallengeNft       struct {
			ContractAddress string      `json:"contractAddress"`
			CreatedAt       string      `json:"createdAt"`
			Description     string      `json:"description"`
			ID              int         `json:"id"`
			ImageURL        string      `json:"imageUrl"`
			Level           interface{} `json:"level"`
			Metadata        string      `json:"metadata"`
			Name            string      `json:"name"`
			Owner           string      `json:"owner"`
			Status          int         `json:"status"`
			Symbol          string      `json:"symbol"`
			TokenID         int         `json:"tokenId"`
			UpdatedAt       string      `json:"updatedAt"`
		} `json:"challengeNft"`
		ChallengeOwner   string `json:"challengeOwner"`
		ChallengeRecords []struct {
			AttackType       int `json:"attackType"`
			ChallengeID      int `json:"challengeId"`
			DefenceType      int `json:"defenceType"`
			ID               int `json:"id"`
			MonsteraID       int `json:"monsteraId"`
			MonsteraLife     int `json:"monsteraLife"`
			MonsteraLifelost int `json:"monsteraLifelost"`
			MonsterbID       int `json:"monsterbId"`
			MonsterbLife     int `json:"monsterbLife"`
			MonsterbLifelost int `json:"monsterbLifelost"`
		} `json:"challengeRecords"`
		ChallengeResult   bool `json:"challengeResult"`
		ChallengedMonster struct {
			Con           int         `json:"con"`
			ConMax        int         `json:"conMax"`
			CreateTime    string      `json:"createTime"`
			Crg           int         `json:"crg"`
			CrgMax        int         `json:"crgMax"`
			Exp           int         `json:"exp"`
			ExpMax        int         `json:"expMax"`
			ID            int         `json:"id"`
			ImageURL      string      `json:"imageUrl"`
			Inte          int         `json:"inte"`
			InteMax       int         `json:"inteMax"`
			Inv           int         `json:"inv"`
			InvMax        int         `json:"invMax"`
			IsPlay        bool        `json:"isPlay"`
			ItemID        int         `json:"itemId"`
			ItemNum       int         `json:"itemNum"`
			LastOwner     string      `json:"lastOwner"`
			Level         int         `json:"level"`
			LevelMax      int         `json:"levelMax"`
			Life          int         `json:"life"`
			LifeLL        int         `json:"lifeLL"`
			Luk           int         `json:"luk"`
			LukMax        int         `json:"lukMax"`
			MonsterUpdate bool        `json:"monsterUpdate"`
			Owner         string      `json:"owner"`
			Race          string      `json:"race"`
			Rarity        string      `json:"rarity"`
			Sca           int         `json:"sca"`
			ScaMax        int         `json:"scaMax"`
			Status        int         `json:"status"`
			Tear          int         `json:"tear"`
			TokenID       interface{} `json:"tokenId"`
			UpdateTime    string      `json:"updateTime"`
			Years         int         `json:"years"`
		} `json:"challengedMonster"`
		ChallengedMonsterID int `json:"challengedMonsterId"`
		ChallengedNft       struct {
			ContractAddress string      `json:"contractAddress"`
			CreatedAt       string      `json:"createdAt"`
			Description     string      `json:"description"`
			ID              int         `json:"id"`
			ImageURL        string      `json:"imageUrl"`
			Level           interface{} `json:"level"`
			Metadata        string      `json:"metadata"`
			Name            string      `json:"name"`
			Owner           string      `json:"owner"`
			Status          int         `json:"status"`
			Symbol          string      `json:"symbol"`
			TokenID         int         `json:"tokenId"`
			UpdatedAt       string      `json:"updatedAt"`
		} `json:"challengedNft"`
		ChallengedOwner string      `json:"challengedOwner"`
		CreateTime      interface{} `json:"createTime"`
		ID              int         `json:"id"`
		MonsterUpdate   bool        `json:"monsterUpdate"`
		UpdateTime      interface{} `json:"updateTime"`
	} `json:"data"`
	ErrorText string `json:"errorText"`
	Message   string `json:"message"`
	Result    int    `json:"result"`
}

func battle(metaIDA, metaIDB int) (bool, error) {
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
		return false, err
	}
	var result BatterResult
	err = resp.ToJSON(&result)
	if err != nil {
		return false, err
	}
	fmt.Println(result.Message)
	if result.Result == 1 {
		return result.Data.ChallengeResult, nil
	}
	if strings.Contains(result.Message, "You didn't pay for the game") {
		return false, noPayErr
	}
	if strings.Contains(result.Message, "Energy") {
		return false, noTearErr
	}
	return false, errors.New("unknown")
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

func updateLevelByID(nftID int) error {
	updateApi := "https://metamon-api.radiocaca.com/usm-api/updateMonster"
	resp, err := req.Post(
		updateApi, req.Param{
			"address": fromAddress,
			"nftId":   nftID,
		}, req.Header{"accesstoken": accessToken},
	)
	if err != nil {
		return err
	}
	result := make(map[string]interface{})
	if err = resp.ToJSON(&result); err != nil {
		return err
	}
	if result["result"].(float64) != -1 {
		fmt.Printf("metamon %d 升级\n", nftID)
		return nil
	}
	return errors.New(fmt.Sprintf("metamon %d 尚未可以升级", nftID))
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
			result := make(map[string]interface{})
			if err = resp.ToJSON(&result); err != nil {
				return err
			}
			if result["result"].(float64) != -1 {
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
