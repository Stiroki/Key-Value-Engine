package block

import (
	"fmt"
	"os"

	"github.com/Stiroki/Key-Value-Engine/cache"
)

type BlockManager struct {
	BlockSize int
	Cache     *cache.LRUCache
}

// Nova instanca BlockManager-a
func NewBlockManager(blockSize int, cache *cache.LRUCache) *BlockManager {
	return &BlockManager{
		BlockSize: blockSize,
		Cache:     cache,
	}
}

// generateCacheKey kreira jedinstveni kljuc za cache na osnovu fajla i indeksa bloka
func (bm *BlockManager) generateCacheKey(filepath string, blockIndex int) string {
	return fmt.Sprintf("%s_%d", filepath, blockIndex)
}

// ReadBlock ucitava blok iz cache-a ili sa diska
func (bm *BlockManager) ReadBlock(filepath string, blockIndex int) ([]byte, error) {
	cacheKey := bm.generateCacheKey(filepath, blockIndex)

	// Provera cache-a
	if cachedData, found := bm.Cache.Get(cacheKey); found {
		return cachedData, nil
	}

	// Ako nije u cache-u, citamo sa diska
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Racunamo odakle počinje blok
	offset := int64(blockIndex * bm.BlockSize)
	buffer := make([]byte, bm.BlockSize)

	// ReadAt cita tacno od specificiranog offset-a
	bytesRead, err := file.ReadAt(buffer, offset)
	if err != nil && err.Error() != "EOF" {
		return nil, err
	}

	// Uzimamo samo onoliko bajtova koliko je stvarno procitano
	actualData := buffer[:bytesRead]

	// Upisujemo u cache
	bm.Cache.Put(cacheKey, actualData)

	return actualData, nil
}

// WriteBlock pize blok na disk i azurira cache
func (bm *BlockManager) WriteBlock(filepath string, blockIndex int, data []byte) error {
	// Otvaramo fajl za pisanje, ako ne postoji kreiramo ga
	file, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	offset := int64(blockIndex * bm.BlockSize)

	// Pisanje na disk
	_, err = file.WriteAt(data, offset)
	if err != nil {
		return err
	}

	// Azuriranje cache-a
	cacheKey := bm.generateCacheKey(filepath, blockIndex)
	bm.Cache.Put(cacheKey, data)

	return nil
}
