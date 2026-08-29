// Package menu holds the nomenclature of the demo venue and turns it into the
// snapshot the platform is given.
package menu

import "avito-kitchen/venue/internal/kitchen"

// Categories of the bakery, in the order it shows them.
var (
	pastry = kitchen.Category{ExternalID: "CAT-PASTRY", Name: "Выпечка", Position: 10}
	bread  = kitchen.Category{ExternalID: "CAT-BREAD", Name: "Хлеб", Position: 20}
	coffee = kitchen.Category{ExternalID: "CAT-COFFEE", Name: "Кофе", Position: 30}
)

// stock returns a limited stock of n portions.
func stock(n int) *int {
	return &n
}

// Dishes is what the bakery sells. It is the venue's own nomenclature: the
// platform learns about it only from the menu the venue uploads.
var Dishes = []kitchen.Dish{
	{
		SKU: "SKU-CROISSANT", Name: "Круассан",
		Description: "Слоёный, с хрустящей корочкой.",
		Price:       19_000, Stock: stock(12), IsAvailable: true,
		Category: pastry, Position: 10,
	},
	{
		SKU: "SKU-CINNAMON", Name: "Синнабон",
		Description: "С корицей и сливочной глазурью.",
		Price:       23_000, Stock: stock(8), IsAvailable: true,
		Category: pastry, Position: 20,
	},
	{
		SKU: "SKU-CHEESECAKE", Name: "Чизкейк",
		Description: "Классический, на песочной основе.",
		Price:       27_000, Stock: stock(6), IsAvailable: true,
		Category: pastry, Position: 30,
	},
	{
		SKU: "SKU-SOURDOUGH", Name: "Хлеб на закваске",
		Description: "Пшеничный, 700 г.",
		Price:       21_000, Stock: stock(10), IsAvailable: true,
		Category: bread, Position: 10,
	},
	{
		SKU: "SKU-BORODINSKY", Name: "Бородинский",
		Description: "Ржаной, с кориандром.",
		Price:       17_000, Stock: stock(10), IsAvailable: true,
		Category: bread, Position: 20,
	},
	{
		SKU: "SKU-BAGUETTE", Name: "Багет",
		Description: "Французский, 250 г.",
		Price:       13_000, Stock: stock(14), IsAvailable: true,
		Category: bread, Position: 30,
	},
	{
		SKU: "SKU-CAPPUCCINO", Name: "Капучино",
		Description: "0,3 л.",
		Price:       22_000, IsAvailable: true,
		Category: coffee, Position: 10,
	},
	{
		SKU: "SKU-AMERICANO", Name: "Американо",
		Description: "0,3 л.",
		Price:       18_000, IsAvailable: true,
		Category: coffee, Position: 20,
	},
}
