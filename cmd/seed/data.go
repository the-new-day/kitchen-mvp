package main

// venue is a demo venue with everything the platform needs to know about it
// before the venue itself connects.
type venue struct {
	id             string
	slug           string
	name           string
	description    string
	address        string
	lat            float64
	lon            float64
	cuisines       []string
	minOrderAmount int64
	deliveryFee    int64
	avgCookMinutes int
	isOpen         bool
	apiKey         string
	menu           []category
}

type category struct {
	externalID string
	name       string
	position   int
	items      []item
}

type item struct {
	externalID  string
	name        string
	description string
	price       int64
	stockQty    *int
}

// venues are the three venues of the closed pilot. The bakery runs its own
// service: it uploads its menu and opens its shift through the partner API,
// so neither is seeded here.
var venues = []venue{
	{
		id:             "0192f4c1-0000-7000-8000-000000000001",
		slug:           "baton",
		name:           "Пекарня «Батон»",
		description:    "Хлеб на закваске и выпечка с утра до вечера.",
		address:        "Москва, Лесная ул., 5",
		lat:            55.777_2,
		lon:            37.588_1,
		cuisines:       []string{"bakery", "desserts", "coffee"},
		minOrderAmount: 50_000,
		deliveryFee:    9_900,
		avgCookMinutes: 15,
		isOpen:         false,
		apiKey:         "vk_demo_bakery_dev",
	},
	{
		id:             "0192f4c1-0000-7000-8000-000000000002",
		slug:           "forno",
		name:           "Пиццерия «Форно»",
		description:    "Неаполитанская пицца на дровах.",
		address:        "Москва, Малая Бронная ул., 12",
		lat:            55.762_3,
		lon:            37.593_4,
		cuisines:       []string{"pizza", "italian"},
		minOrderAmount: 80_000,
		deliveryFee:    14_900,
		avgCookMinutes: 25,
		isOpen:         true,
		apiKey:         "vk_demo_pizza_dev",
		menu: []category{
			{
				externalID: "CAT-PIZZA",
				name:       "Пицца",
				position:   10,
				items: []item{
					{externalID: "SKU-MARGHERITA", name: "Маргарита", description: "Томаты, моцарелла, базилик.", price: 59_000, stockQty: new(20)},
					{externalID: "SKU-PEPPERONI", name: "Пепперони", description: "Острая салями, моцарелла.", price: 69_000, stockQty: new(15)},
					{externalID: "SKU-QUATTRO", name: "Четыре сыра", description: "Моцарелла, горгонзола, пармезан, скаморца.", price: 79_000, stockQty: new(10)},
				},
			},
			{
				externalID: "CAT-PASTA",
				name:       "Паста",
				position:   20,
				items: []item{
					{externalID: "SKU-CARBONARA", name: "Карбонара", description: "Гуанчале, желток, пекорино.", price: 64_000, stockQty: new(12)},
					{externalID: "SKU-ARRABBIATA", name: "Арраббьята", description: "Томаты, чеснок, перец чили.", price: 52_000},
				},
			},
			{
				externalID: "CAT-DRINKS",
				name:       "Напитки",
				position:   30,
				items: []item{
					{externalID: "SKU-LEMONADE", name: "Домашний лимонад", description: "0,33 л.", price: 19_000},
					{externalID: "SKU-ESPRESSO", name: "Эспрессо", description: "", price: 15_000},
				},
			},
		},
	},
	{
		id:             "0192f4c1-0000-7000-8000-000000000003",
		slug:           "kabuki",
		name:           "Суши «Кабуки»",
		description:    "Роллы и сеты, готовим при заказе.",
		address:        "Москва, Пятницкая ул., 41",
		lat:            55.735_9,
		lon:            37.626_8,
		cuisines:       []string{"sushi", "japanese", "asian"},
		minOrderAmount: 120_000,
		deliveryFee:    19_900,
		avgCookMinutes: 35,
		isOpen:         true,
		apiKey:         "vk_demo_sushi_dev",
		menu: []category{
			{
				externalID: "CAT-ROLLS",
				name:       "Роллы",
				position:   10,
				items: []item{
					{externalID: "SKU-PHILADELPHIA", name: "Филадельфия", description: "Лосось, сливочный сыр, огурец.", price: 89_000, stockQty: new(8)},
					{externalID: "SKU-CALIFORNIA", name: "Калифорния", description: "Краб, авокадо, икра тобико.", price: 74_000, stockQty: new(8)},
					{externalID: "SKU-UNAGI", name: "Унаги маки", description: "Копчёный угорь, соус унаги.", price: 96_000, stockQty: new(4)},
				},
			},
			{
				externalID: "CAT-SETS",
				name:       "Сеты",
				position:   20,
				items: []item{
					{externalID: "SKU-SET-DUO", name: "Сет «Дуэт»", description: "32 кусочка, два вида роллов.", price: 189_000, stockQty: new(3)},
				},
			},
			{
				externalID: "CAT-SOUPS",
				name:       "Супы",
				position:   30,
				items: []item{
					{externalID: "SKU-MISO", name: "Мисо-суп", description: "Тофу, водоросли вакамэ.", price: 29_000},
				},
			},
		},
	},
}
