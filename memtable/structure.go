package memtable

import "github.com/Stiroki/Key-Value-Engine/model"

type Structure interface {
	Put(record *model.Record)             // Dodaje ili azurira zapis
	Get(key string) (*model.Record, bool) // Vraca sam zapis i bool
	GetAll() []*model.Record              // Vraca sve zapise
	Size() int                            // Trenutni broj zapisa
	Clear()                               // Cisti sve zapise nakon flush-a
}
