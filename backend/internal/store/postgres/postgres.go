package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"kasirinaja/backend/internal/domain"
	"kasirinaja/backend/internal/store"
	"kasirinaja/backend/internal/xid"
)

type Store struct {
	db *sql.DB
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	db.SetMaxIdleConns(8)
	db.SetMaxOpenConns(30)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ListProducts(ctx context.Context) ([]domain.Product, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sku, name, category, price_cents, margin_rate, active
		FROM products
		WHERE active = true
		ORDER BY category, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := make([]domain.Product, 0, 128)
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.SKU, &p.Name, &p.Category, &p.PriceCents, &p.MarginRate, &p.Active); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

func (s *Store) CreateProduct(ctx context.Context, product domain.Product) (*domain.Product, error) {
	if product.SKU == "" || product.Name == "" || product.Category == "" || product.PriceCents < 1 {
		return nil, store.ErrInvalidTransaction
	}
	if product.MarginRate < 0 || product.MarginRate > 1 {
		return nil, store.ErrInvalidTransaction
	}

	product.Active = true
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO products (sku, name, category, price_cents, margin_rate, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,now(),now())
	`, product.SKU, product.Name, product.Category, product.PriceCents, product.MarginRate, product.Active)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, store.ErrInvalidTransaction
		}
		return nil, err
	}

	created := product
	return &created, nil
}

func (s *Store) GetProductBySKU(ctx context.Context, sku string) (*domain.Product, error) {
	var product domain.Product
	err := s.db.QueryRowContext(ctx, `
		SELECT sku, name, category, price_cents, margin_rate, active
		FROM products
		WHERE sku = $1
	`, sku).Scan(&product.SKU, &product.Name, &product.Category, &product.PriceCents, &product.MarginRate, &product.Active)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &product, nil
}

func (s *Store) UpdateProduct(ctx context.Context, product domain.Product) (*domain.Product, error) {
	if product.SKU == "" || product.Name == "" || product.Category == "" || product.PriceCents < 1 {
		return nil, store.ErrInvalidTransaction
	}
	if product.MarginRate < 0 || product.MarginRate > 1 {
		return nil, store.ErrInvalidTransaction
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE products
		SET name = $2, category = $3, price_cents = $4, margin_rate = $5, active = $6, updated_at = now()
		WHERE sku = $1
	`, product.SKU, product.Name, product.Category, product.PriceCents, product.MarginRate, product.Active)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, store.ErrNotFound
	}

	updated := product
	return &updated, nil
}

func (s *Store) CreatePriceHistory(ctx context.Context, entry domain.ProductPriceHistory) error {
	if entry.ID == "" {
		entry.ID = xid.New("ph")
	}
	if entry.ChangedAt.IsZero() {
		entry.ChangedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO product_price_history (id, sku, old_price_cents, new_price_cents, changed_by, changed_at)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, entry.ID, entry.SKU, entry.OldPriceCents, entry.NewPriceCents, entry.ChangedBy, entry.ChangedAt)
	return err
}

func (s *Store) ListPriceHistory(ctx context.Context, sku string, limit int) ([]domain.ProductPriceHistory, error) {
	if limit < 1 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, sku, old_price_cents, new_price_cents, changed_by, changed_at
		FROM product_price_history
		WHERE sku = $1
		ORDER BY changed_at DESC
		LIMIT $2
	`, sku, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := make([]domain.ProductPriceHistory, 0, limit)
	for rows.Next() {
		var entry domain.ProductPriceHistory
		if err := rows.Scan(&entry.ID, &entry.SKU, &entry.OldPriceCents, &entry.NewPriceCents, &entry.ChangedBy, &entry.ChangedAt); err != nil {
			return nil, err
		}
		entry.ChangedAt = entry.ChangedAt.UTC()
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return history, nil
}

func (s *Store) GetProductsBySKUs(ctx context.Context, skus []string) (map[string]domain.Product, error) {
	result := make(map[string]domain.Product, len(skus))
	if len(skus) == 0 {
		return result, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT sku, name, category, price_cents, margin_rate, active
		FROM products
		WHERE active = true AND sku = ANY($1)
	`, skus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.SKU, &p.Name, &p.Category, &p.PriceCents, &p.MarginRate, &p.Active); err != nil {
			return nil, err
		}
		result[p.SKU] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Store) GetStockMap(ctx context.Context, storeID string, skus []string) (map[string]int, error) {
	stockMap := make(map[string]int, len(skus))
	if len(skus) == 0 {
		return stockMap, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT sku, qty
		FROM inventory_stocks
		WHERE store_id = $1 AND sku = ANY($2)
	`, storeID, skus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sku string
		var qty int
		if err := rows.Scan(&sku, &qty); err != nil {
			return nil, err
		}
		stockMap[sku] = qty
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, sku := range skus {
		if _, ok := stockMap[sku]; !ok {
			stockMap[sku] = 0
		}
	}

	return stockMap, nil
}

func (s *Store) SetStock(ctx context.Context, storeID string, sku string, qty int) error {
	if sku == "" || qty < 0 {
		return store.ErrInvalidTransaction
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO inventory_stocks (store_id, sku, qty, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (store_id, sku)
		DO UPDATE SET qty = EXCLUDED.qty, updated_at = now()
	`, storeID, sku, qty)
	return err
}

func (s *Store) CreateInventoryLot(ctx context.Context, lot domain.InventoryLot) (*domain.InventoryLot, error) {
	if strings.TrimSpace(lot.StoreID) == "" || strings.TrimSpace(lot.SKU) == "" || lot.QtyReceived < 1 || lot.CostCents < 1 {
		return nil, store.ErrInvalidTransaction
	}
	if lot.ID == "" {
		lot.ID = xid.New("lot")
	}
	lot.SKU = strings.ToUpper(strings.TrimSpace(lot.SKU))
	lot.LotCode = strings.TrimSpace(lot.LotCode)
	if lot.LotCode == "" {
		lot.LotCode = "MANUAL-" + lot.ID
	}
	if lot.SourceType == "" {
		lot.SourceType = "manual"
	}
	if lot.ReceivedAt.IsZero() {
		lot.ReceivedAt = time.Now().UTC()
	}
	if lot.QtyAvailable < 0 || lot.QtyAvailable > lot.QtyReceived {
		return nil, store.ErrInvalidTransaction
	}
	if lot.QtyAvailable == 0 {
		lot.QtyAvailable = lot.QtyReceived
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_lots (
			id, store_id, sku, lot_code, expiry_date, qty_received, qty_available,
			cost_cents, source_type, source_id, notes, received_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
	`, lot.ID, lot.StoreID, lot.SKU, lot.LotCode, nullDate(lot.ExpiryDate), lot.QtyReceived, lot.QtyAvailable, lot.CostCents, lot.SourceType, nullIfEmpty(lot.SourceID), strings.TrimSpace(lot.Notes), lot.ReceivedAt)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_stocks (store_id, sku, qty, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (store_id, sku)
		DO UPDATE SET qty = inventory_stocks.qty + EXCLUDED.qty, updated_at = now()
	`, lot.StoreID, lot.SKU, lot.QtyAvailable)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	created := lot
	return &created, nil
}

func (s *Store) ListInventoryLots(ctx context.Context, storeID string, sku string, includeExpired bool, limit int) ([]domain.InventoryLot, error) {
	if limit < 1 {
		limit = 200
	}
	sku = strings.ToUpper(strings.TrimSpace(sku))

	query := `
		SELECT id, store_id, sku, lot_code, expiry_date, qty_received, qty_available,
			cost_cents, source_type, source_id, notes, received_at
		FROM inventory_lots
		WHERE ($1 = '' OR store_id = $1)
			AND ($2 = '' OR sku = $2)
	`
	if !includeExpired {
		query += ` AND (expiry_date IS NULL OR expiry_date >= CURRENT_DATE)`
	}
	query += `
		ORDER BY expiry_date ASC NULLS LAST, received_at ASC
		LIMIT $3
	`

	rows, err := s.db.QueryContext(ctx, query, storeID, sku, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lots := make([]domain.InventoryLot, 0, limit)
	for rows.Next() {
		var lot domain.InventoryLot
		var expiry sql.NullTime
		var sourceID sql.NullString
		if err := rows.Scan(&lot.ID, &lot.StoreID, &lot.SKU, &lot.LotCode, &expiry, &lot.QtyReceived, &lot.QtyAvailable, &lot.CostCents, &lot.SourceType, &sourceID, &lot.Notes, &lot.ReceivedAt); err != nil {
			return nil, err
		}
		lot.ReceivedAt = lot.ReceivedAt.UTC()
		if expiry.Valid {
			e := time.Date(expiry.Time.UTC().Year(), expiry.Time.UTC().Month(), expiry.Time.UTC().Day(), 0, 0, 0, 0, time.UTC)
			lot.ExpiryDate = &e
		}
		if sourceID.Valid {
			lot.SourceID = sourceID.String
		}
		lots = append(lots, lot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return lots, nil
}

func (s *Store) IncreaseStock(ctx context.Context, storeID string, adjustments []domain.StockAdjustment) error {
	if len(adjustments) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, adj := range adjustments {
		if adj.Qty < 1 {
			continue
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_stocks (store_id, sku, qty, updated_at)
			VALUES ($1,$2,$3,now())
			ON CONFLICT (store_id, sku)
			DO UPDATE SET qty = inventory_stocks.qty + EXCLUDED.qty, updated_at = now()
		`, storeID, adj.SKU, adj.Qty)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetAssociationPairs(ctx context.Context, sourceSKUs []string) ([]domain.AssociationPair, error) {
	pairs := make([]domain.AssociationPair, 0)
	if len(sourceSKUs) == 0 {
		return pairs, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT source_sku, target_sku, affinity_score
		FROM association_item_pairs
		WHERE source_sku = ANY($1)
	`, sourceSKUs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pair domain.AssociationPair
		if err := rows.Scan(&pair.SourceSKU, &pair.TargetSKU, &pair.Affinity); err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return pairs, nil
}

func (s *Store) FindTransactionByIdempotency(ctx context.Context, key string) (*domain.Transaction, error) {
	return s.findTransaction(ctx, "idempotency_key", key)
}

func (s *Store) FindTransactionByID(ctx context.Context, id string) (*domain.Transaction, error) {
	return s.findTransaction(ctx, "id", id)
}

func (s *Store) findTransaction(ctx context.Context, column string, value string) (*domain.Transaction, error) {
	if column != "id" && column != "idempotency_key" {
		return nil, fmt.Errorf("unsupported lookup column")
	}

	var tx domain.Transaction
	var recommendationSKU sql.NullString
	var shiftID sql.NullString
	var paymentReference sql.NullString
	var voidReason sql.NullString
	var voidedAt sql.NullTime

	query := fmt.Sprintf(`
		SELECT id, store_id, terminal_id, COALESCE(shift_id,''), idempotency_key,
			payment_method, payment_reference, subtotal_cents, discount_cents,
			tax_rate_percent, tax_cents, total_cents, cash_received_cents, change_cents,
			status, recommendation_shown, recommendation_accepted, recommendation_sku,
			void_reason, voided_at, created_at
		FROM transactions
		WHERE %s = $1
	`, column)

	err := s.db.QueryRowContext(ctx, query, value).Scan(
		&tx.ID,
		&tx.StoreID,
		&tx.TerminalID,
		&shiftID,
		&tx.IdempotencyKey,
		&tx.PaymentMethod,
		&paymentReference,
		&tx.SubtotalCents,
		&tx.DiscountCents,
		&tx.TaxRatePercent,
		&tx.TaxCents,
		&tx.TotalCents,
		&tx.CashReceivedCents,
		&tx.ChangeCents,
		&tx.Status,
		&tx.RecommendationShown,
		&tx.RecommendationAccepted,
		&recommendationSKU,
		&voidReason,
		&voidedAt,
		&tx.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	if shiftID.Valid {
		tx.ShiftID = shiftID.String
	}
	if paymentReference.Valid {
		tx.PaymentReference = paymentReference.String
	}
	if recommendationSKU.Valid {
		tx.RecommendationSKU = recommendationSKU.String
	}
	if voidReason.Valid {
		tx.VoidReason = voidReason.String
	}
	if voidedAt.Valid {
		at := voidedAt.Time.UTC()
		tx.VoidedAt = &at
	}
	tx.CreatedAt = tx.CreatedAt.UTC()

	rows, err := s.db.QueryContext(ctx, `
		SELECT sku, qty, unit_price_cents, margin_rate
		FROM transaction_items
		WHERE transaction_id = $1
		ORDER BY id ASC
	`, tx.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.TransactionLine, 0, 8)
	for rows.Next() {
		var item domain.TransactionLine
		if err := rows.Scan(&item.SKU, &item.Qty, &item.UnitPriceCents, &item.MarginRate); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tx.Items = items

	return &tx, nil
}

func (s *Store) CreateCheckout(ctx context.Context, tx domain.Transaction) (*domain.Transaction, error) {
	if tx.IdempotencyKey == "" {
		return nil, store.ErrInvalidTransaction
	}
	if len(tx.Items) == 0 {
		return nil, store.ErrInvalidTransaction
	}

	pgTx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = pgTx.Rollback() }()

	skus := uniqueSKUs(tx.Items)
	if len(skus) == 0 {
		return nil, store.ErrInvalidTransaction
	}

	productRows, err := pgTx.QueryContext(ctx, `
		SELECT sku, price_cents, margin_rate
		FROM products
		WHERE active = true AND sku = ANY($1)
	`, skus)
	if err != nil {
		return nil, err
	}
	productMap := make(map[string]domain.Product, len(skus))
	for productRows.Next() {
		var sku string
		var priceCents int64
		var marginRate float64
		if err := productRows.Scan(&sku, &priceCents, &marginRate); err != nil {
			_ = productRows.Close()
			return nil, err
		}
		productMap[sku] = domain.Product{SKU: sku, PriceCents: priceCents, MarginRate: marginRate, Active: true}
	}
	if err := productRows.Err(); err != nil {
		_ = productRows.Close()
		return nil, err
	}
	_ = productRows.Close()

	stockRows, err := pgTx.QueryContext(ctx, `
		SELECT sku, qty
		FROM inventory_stocks
		WHERE store_id = $1 AND sku = ANY($2)
		FOR UPDATE
	`, tx.StoreID, skus)
	if err != nil {
		return nil, err
	}
	stockMap := make(map[string]int, len(skus))
	for stockRows.Next() {
		var sku string
		var qty int
		if err := stockRows.Scan(&sku, &qty); err != nil {
			_ = stockRows.Close()
			return nil, err
		}
		stockMap[sku] = qty
	}
	if err := stockRows.Err(); err != nil {
		_ = stockRows.Close()
		return nil, err
	}
	_ = stockRows.Close()

	subtotalCents := int64(0)
	recomputedItems := make([]domain.TransactionLine, 0, len(tx.Items))
	today := nowDateUTC(time.Now().UTC())
	for _, item := range tx.Items {
		if item.Qty < 1 {
			return nil, store.ErrInvalidTransaction
		}

		product, exists := productMap[item.SKU]
		if !exists {
			return nil, fmt.Errorf("sku %s unavailable", item.SKU)
		}

		stockQty, exists := stockMap[item.SKU]
		if !exists || stockQty < item.Qty {
			return nil, store.ErrInsufficientStock
		}
		lotRows, err := pgTx.QueryContext(ctx, `
			SELECT id, expiry_date, qty_available
			FROM inventory_lots
			WHERE store_id = $1 AND sku = $2 AND qty_available > 0
			ORDER BY expiry_date ASC NULLS LAST, received_at ASC
			FOR UPDATE
		`, tx.StoreID, item.SKU)
		if err != nil {
			return nil, err
		}
		type lotState struct {
			id       string
			expiry   *time.Time
			available int
		}
		lots := make([]lotState, 0, 8)
		for lotRows.Next() {
			var lotID string
			var expiry sql.NullTime
			var available int
			if err := lotRows.Scan(&lotID, &expiry, &available); err != nil {
				_ = lotRows.Close()
				return nil, err
			}
			var expiryDate *time.Time
			if expiry.Valid {
				e := nowDateUTC(expiry.Time.UTC())
				expiryDate = &e
			}
			lots = append(lots, lotState{id: lotID, expiry: expiryDate, available: available})
		}
		if err := lotRows.Err(); err != nil {
			_ = lotRows.Close()
			return nil, err
		}
		_ = lotRows.Close()
		if len(lots) > 0 {
			availableFromLots := 0
			for _, lot := range lots {
				if lot.expiry != nil && lot.expiry.Before(today) {
					continue
				}
				availableFromLots += lot.available
			}
			if availableFromLots < item.Qty {
				return nil, store.ErrInsufficientStock
			}
			remainingFromLots := item.Qty
			for _, lot := range lots {
				if remainingFromLots == 0 {
					break
				}
				if lot.available < 1 {
					continue
				}
				if lot.expiry != nil && lot.expiry.Before(today) {
					continue
				}
				used := remainingFromLots
				if used > lot.available {
					used = lot.available
				}
				_, err = pgTx.ExecContext(ctx, `
					UPDATE inventory_lots
					SET qty_available = qty_available - $1, updated_at = now()
					WHERE id = $2
				`, used, lot.id)
				if err != nil {
					return nil, err
				}
				remainingFromLots -= used
			}
			if remainingFromLots > 0 {
				return nil, store.ErrInsufficientStock
			}
		}

		_, err = pgTx.ExecContext(ctx, `
			UPDATE inventory_stocks
			SET qty = qty - $1, updated_at = now()
			WHERE store_id = $2 AND sku = $3
		`, item.Qty, tx.StoreID, item.SKU)
		if err != nil {
			return nil, err
		}

		recomputedItems = append(recomputedItems, domain.TransactionLine{
			SKU:            item.SKU,
			Qty:            item.Qty,
			UnitPriceCents: product.PriceCents,
			MarginRate:     product.MarginRate,
		})
		subtotalCents += product.PriceCents * int64(item.Qty)
	}

	if tx.DiscountCents < 0 || tx.DiscountCents > subtotalCents {
		return nil, store.ErrInvalidTransaction
	}
	if tx.TaxRatePercent < 0 || tx.TaxRatePercent > 100 {
		return nil, store.ErrInvalidTransaction
	}

	taxBase := subtotalCents - tx.DiscountCents
	taxCents := int64(math.Round(float64(taxBase) * tx.TaxRatePercent / 100))
	totalCents := taxBase + taxCents

	if tx.PaymentMethod == "cash" {
		if tx.CashReceivedCents < totalCents {
			return nil, store.ErrInvalidTransaction
		}
		tx.ChangeCents = tx.CashReceivedCents - totalCents
	} else {
		tx.ChangeCents = 0
	}

	tx.SubtotalCents = subtotalCents
	tx.TaxCents = taxCents
	tx.TotalCents = totalCents
	tx.Items = recomputedItems
	if tx.ID == "" {
		tx.ID = xid.New("tx")
	}
	if tx.CreatedAt.IsZero() {
		tx.CreatedAt = time.Now().UTC()
	}
	if tx.Status == "" {
		tx.Status = domain.TxStatusPaid
	}

	_, err = pgTx.ExecContext(ctx, `
		INSERT INTO transactions (
			id, store_id, terminal_id, shift_id, idempotency_key, payment_method,
			payment_reference, subtotal_cents, discount_cents, tax_rate_percent, tax_cents,
			total_cents, cash_received_cents, change_cents, status,
			recommendation_shown, recommendation_accepted, recommendation_sku,
			void_reason, voided_at, created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`, tx.ID, tx.StoreID, tx.TerminalID, nullIfEmpty(tx.ShiftID), tx.IdempotencyKey, tx.PaymentMethod,
		nullIfEmpty(tx.PaymentReference), tx.SubtotalCents, tx.DiscountCents, tx.TaxRatePercent,
		tx.TaxCents, tx.TotalCents, tx.CashReceivedCents, tx.ChangeCents, tx.Status,
		tx.RecommendationShown, tx.RecommendationAccepted, nullIfEmpty(tx.RecommendationSKU),
		nullIfEmpty(tx.VoidReason), nullTime(tx.VoidedAt), tx.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			existing, lookupErr := s.FindTransactionByIdempotency(ctx, tx.IdempotencyKey)
			if lookupErr == nil {
				return existing, nil
			}
		}
		return nil, err
	}

	for _, item := range tx.Items {
		_, err := pgTx.ExecContext(ctx, `
			INSERT INTO transaction_items (transaction_id, sku, qty, unit_price_cents, margin_rate)
			VALUES ($1,$2,$3,$4,$5)
		`, tx.ID, item.SKU, item.Qty, item.UnitPriceCents, item.MarginRate)
		if err != nil {
			return nil, err
		}
	}

	if err := pgTx.Commit(); err != nil {
		return nil, err
	}

	return &tx, nil
}

func (s *Store) VoidTransaction(ctx context.Context, id string, reason string, at time.Time) (*domain.Transaction, error) {
	pgTx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = pgTx.Rollback() }()

	var tx domain.Transaction
	err = pgTx.QueryRowContext(ctx, `
		SELECT id, store_id, status
		FROM transactions
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&tx.ID, &tx.StoreID, &tx.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	if tx.Status != domain.TxStatusPaid {
		return nil, store.ErrInvalidTransaction
	}

	itemRows, err := pgTx.QueryContext(ctx, `
		SELECT sku, qty
		FROM transaction_items
		WHERE transaction_id = $1
	`, id)
	if err != nil {
		return nil, err
	}
	items := make([]domain.TransactionLine, 0, 8)
	for itemRows.Next() {
		var item domain.TransactionLine
		if err := itemRows.Scan(&item.SKU, &item.Qty); err != nil {
			_ = itemRows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := itemRows.Err(); err != nil {
		_ = itemRows.Close()
		return nil, err
	}
	_ = itemRows.Close()

	_, err = pgTx.ExecContext(ctx, `
		UPDATE transactions
		SET status = $2, void_reason = $3, voided_at = $4
		WHERE id = $1 AND status = $5
	`, id, domain.TxStatusVoided, reason, at, domain.TxStatusPaid)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		lotID := xid.New("lot")
		lotCode := "VOID-" + id
		_, err := pgTx.ExecContext(ctx, `
			INSERT INTO inventory_stocks (store_id, sku, qty, updated_at)
			VALUES ($1,$2,$3,now())
			ON CONFLICT (store_id, sku)
			DO UPDATE SET qty = inventory_stocks.qty + EXCLUDED.qty, updated_at = now()
		`, tx.StoreID, item.SKU, item.Qty)
		if err != nil {
			return nil, err
		}
		_, err = pgTx.ExecContext(ctx, `
			INSERT INTO inventory_lots (
				id, store_id, sku, lot_code, expiry_date, qty_received, qty_available,
				cost_cents, source_type, source_id, notes, received_at, updated_at
			)
			VALUES ($1,$2,$3,$4,NULL,$5,$6,$7,'void',$8,$9,$10,now())
		`, lotID, tx.StoreID, item.SKU, lotCode, item.Qty, item.Qty, maxInt64(1, item.UnitPriceCents), id, "auto restock from void", at)
		if err != nil {
			return nil, err
		}
	}

	if err := pgTx.Commit(); err != nil {
		return nil, err
	}

	tx.Status = domain.TxStatusVoided
	tx.VoidReason = reason
	tx.VoidedAt = &at
	return &tx, nil
}

func (s *Store) CreateRefund(ctx context.Context, refund domain.Refund) (*domain.Refund, error) {
	if refund.ID == "" {
		refund.ID = xid.New("refund")
	}
	if refund.CreatedAt.IsZero() {
		refund.CreatedAt = time.Now().UTC()
	}
	if refund.Status == "" {
		refund.Status = domain.TxStatusRefunded
	}

	pgTx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = pgTx.Rollback() }()

	var transactionTotal int64
	var transactionStatus string
	err = pgTx.QueryRowContext(ctx, `
		SELECT total_cents, status
		FROM transactions
		WHERE id = $1
		FOR UPDATE
	`, refund.OriginalTransactionID).Scan(&transactionTotal, &transactionStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	if transactionStatus == domain.TxStatusVoided || transactionStatus == domain.TxStatusRefunded {
		return nil, store.ErrInvalidTransaction
	}

	refundedSoFar := int64(0)
	rows, err := pgTx.QueryContext(ctx, `
		SELECT amount_cents
		FROM refunds
		WHERE original_transaction_id = $1 AND status = $2
		FOR UPDATE
	`, refund.OriginalTransactionID, domain.TxStatusRefunded)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var amount int64
		if err := rows.Scan(&amount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		refundedSoFar += amount
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	remaining := transactionTotal - refundedSoFar
	if refund.AmountCents > remaining {
		return nil, store.ErrInvalidTransaction
	}

	_, err = pgTx.ExecContext(ctx, `
		INSERT INTO refunds (id, original_transaction_id, reason, amount_cents, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, refund.ID, refund.OriginalTransactionID, refund.Reason, refund.AmountCents, refund.Status, refund.CreatedAt)
	if err != nil {
		return nil, err
	}

	nextStatus := domain.TxStatusPaid
	if refundedSoFar+refund.AmountCents >= transactionTotal {
		nextStatus = domain.TxStatusRefunded
	}

	_, err = pgTx.ExecContext(ctx, `
		UPDATE transactions
		SET status = $2
		WHERE id = $1
	`, refund.OriginalTransactionID, nextStatus)
	if err != nil {
		return nil, err
	}

	if err := pgTx.Commit(); err != nil {
		return nil, err
	}

	return &refund, nil
}

func (s *Store) GetReturnedQtyByTransaction(ctx context.Context, transactionID string) (map[string]int, error) {
	result := make(map[string]int)
	rows, err := s.db.QueryContext(ctx, `
		SELECT iri.sku, COALESCE(SUM(iri.qty), 0)::int
		FROM item_returns ir
		JOIN item_return_items iri ON iri.item_return_id = ir.id
		WHERE ir.original_transaction_id = $1 AND iri.kind = 'return'
		GROUP BY iri.sku
	`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sku string
		var qty int
		if err := rows.Scan(&sku, &qty); err != nil {
			return nil, err
		}
		result[sku] = qty
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) CreateItemReturn(ctx context.Context, itemReturn domain.ItemReturn) (*domain.ItemReturn, error) {
	if itemReturn.ID == "" {
		itemReturn.ID = xid.New("ret")
	}
	if itemReturn.CreatedAt.IsZero() {
		itemReturn.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(itemReturn.OriginalTransactionID) == "" || len(itemReturn.ReturnItems) == 0 {
		return nil, store.ErrInvalidTransaction
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO item_returns (
			id, store_id, original_transaction_id, mode, reason, refund_amount_cents,
			exchange_transaction_id, additional_payment_cents, processed_by, created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, itemReturn.ID, itemReturn.StoreID, itemReturn.OriginalTransactionID, itemReturn.Mode, itemReturn.Reason, itemReturn.RefundAmountCents, nullIfEmpty(itemReturn.ExchangeTransactionID), itemReturn.AdditionalPaymentCents, itemReturn.ProcessedBy, itemReturn.CreatedAt)
	if err != nil {
		return nil, err
	}
	for _, line := range itemReturn.ReturnItems {
		if line.Qty < 1 {
			return nil, store.ErrInvalidTransaction
		}
		if line.UnitPriceCents < 1 {
			return nil, store.ErrInvalidTransaction
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO item_return_items (item_return_id, sku, qty, unit_price_cents, kind)
			VALUES ($1,$2,$3,$4,'return')
		`, itemReturn.ID, line.SKU, line.Qty, line.UnitPriceCents)
		if err != nil {
			return nil, err
		}
	}
	for _, line := range itemReturn.ExchangeItems {
		if line.Qty < 1 {
			return nil, store.ErrInvalidTransaction
		}
		if line.UnitPriceCents < 1 {
			return nil, store.ErrInvalidTransaction
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO item_return_items (item_return_id, sku, qty, unit_price_cents, kind)
			VALUES ($1,$2,$3,$4,'exchange')
		`, itemReturn.ID, line.SKU, line.Qty, line.UnitPriceCents)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	created := itemReturn
	return &created, nil
}

func (s *Store) CreateRecommendationEvent(ctx context.Context, event domain.RecommendationEvent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO recommendation_events (
			id, store_id, terminal_id, transaction_id,
			sku, action, reason_code, confidence, latency_ms, created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`,
		xid.New("reco"),
		event.StoreID,
		event.TerminalID,
		nullIfEmpty(event.TransactionID),
		nullIfEmpty(event.SKU),
		event.Action,
		nullIfEmpty(event.ReasonCode),
		event.Confidence,
		event.LatencyMS,
		event.CreatedAt,
	)
	return err
}

func (s *Store) GetAttachMetrics(ctx context.Context, storeID string, from time.Time, to time.Time) (domain.AttachMetrics, error) {
	var metrics domain.AttachMetrics
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)::bigint,
			COALESCE(SUM(CASE WHEN recommendation_accepted THEN 1 ELSE 0 END),0)::bigint
		FROM transactions
		WHERE store_id = $1 AND created_at BETWEEN $2 AND $3 AND status <> $4
	`, storeID, from, to, domain.TxStatusVoided).Scan(&metrics.Transactions, &metrics.Accepted)
	if err != nil {
		return metrics, err
	}

	if metrics.Transactions > 0 {
		metrics.AttachRate = (float64(metrics.Accepted) / float64(metrics.Transactions)) * 100
	}

	return metrics, nil
}

func (s *Store) GetDailyReport(ctx context.Context, storeID string, from time.Time, to time.Time) (domain.DailyReport, error) {
	report := domain.DailyReport{
		StoreID:    storeID,
		ByPayment:  make([]domain.DailyReportPayment, 0, 4),
		ByTerminal: make([]domain.DailyReportTerminal, 0, 8),
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)::bigint,
			COALESCE(SUM(subtotal_cents),0)::bigint,
			COALESCE(SUM(discount_cents),0)::bigint,
			COALESCE(SUM(tax_cents),0)::bigint,
			COALESCE(SUM(total_cents),0)::bigint
		FROM transactions
		WHERE store_id = $1
			AND created_at >= $2
			AND created_at < $3
			AND status <> $4
	`, storeID, from, to, domain.TxStatusVoided).Scan(
		&report.Transactions,
		&report.GrossSalesCents,
		&report.DiscountCents,
		&report.TaxCents,
		&report.NetSalesCents,
	)
	if err != nil {
		return report, err
	}

	var refundedCents int64
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(r.amount_cents),0)::bigint
		FROM refunds r
		JOIN transactions t ON t.id = r.original_transaction_id
		WHERE t.store_id = $1
			AND r.created_at >= $2
			AND r.created_at < $3
			AND r.status = $4
	`, storeID, from, to, domain.TxStatusRefunded).Scan(&refundedCents)
	if err != nil {
		return report, err
	}
	if refundedCents > 0 {
		report.NetSalesCents -= refundedCents
		if report.NetSalesCents < 0 {
			report.NetSalesCents = 0
		}
	}

	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(ROUND((ti.unit_price_cents * ti.qty) * ti.margin_rate)),0)::bigint
		FROM transaction_items ti
		JOIN transactions t ON t.id = ti.transaction_id
		WHERE t.store_id = $1
			AND t.created_at >= $2
			AND t.created_at < $3
			AND t.status <> $4
	`, storeID, from, to, domain.TxStatusVoided).Scan(&report.EstimatedMarginCents)
	if err != nil {
		return report, err
	}

	paymentRows, err := s.db.QueryContext(ctx, `
		SELECT payment_method, COUNT(*)::bigint, COALESCE(SUM(total_cents),0)::bigint
		FROM transactions
		WHERE store_id = $1
			AND created_at >= $2
			AND created_at < $3
			AND status <> $4
		GROUP BY payment_method
		ORDER BY payment_method
	`, storeID, from, to, domain.TxStatusVoided)
	if err != nil {
		return report, err
	}
	for paymentRows.Next() {
		var row domain.DailyReportPayment
		if err := paymentRows.Scan(&row.PaymentMethod, &row.Transactions, &row.TotalCents); err != nil {
			_ = paymentRows.Close()
			return report, err
		}
		report.ByPayment = append(report.ByPayment, row)
	}
	if err := paymentRows.Err(); err != nil {
		_ = paymentRows.Close()
		return report, err
	}
	_ = paymentRows.Close()

	terminalRows, err := s.db.QueryContext(ctx, `
		SELECT terminal_id, COUNT(*)::bigint, COALESCE(SUM(total_cents),0)::bigint
		FROM transactions
		WHERE store_id = $1
			AND created_at >= $2
			AND created_at < $3
			AND status <> $4
		GROUP BY terminal_id
		ORDER BY terminal_id
	`, storeID, from, to, domain.TxStatusVoided)
	if err != nil {
		return report, err
	}
	for terminalRows.Next() {
		var row domain.DailyReportTerminal
		if err := terminalRows.Scan(&row.TerminalID, &row.Transactions, &row.TotalCents); err != nil {
			_ = terminalRows.Close()
			return report, err
		}
		report.ByTerminal = append(report.ByTerminal, row)
	}
	if err := terminalRows.Err(); err != nil {
		_ = terminalRows.Close()
		return report, err
	}
	_ = terminalRows.Close()

	return report, nil
}

func (s *Store) CreateAuditLog(ctx context.Context, entry domain.AuditLog) error {
	if entry.ID == "" {
		entry.ID = xid.New("audit")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id, store_id, actor_username, actor_role, action, entity_type, entity_id, detail, created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, entry.ID, entry.StoreID, entry.ActorUsername, entry.ActorRole, entry.Action, entry.EntityType, entry.EntityID, entry.Detail, entry.CreatedAt)
	return err
}

func (s *Store) ListAuditLogs(ctx context.Context, storeID string, from time.Time, to time.Time, limit int) ([]domain.AuditLog, error) {
	if limit < 1 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, store_id, actor_username, actor_role, action, entity_type, entity_id, detail, created_at
		FROM audit_logs
		WHERE store_id = $1
			AND created_at >= $2
			AND created_at < $3
		ORDER BY created_at DESC
		LIMIT $4
	`, storeID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]domain.AuditLog, 0, limit)
	for rows.Next() {
		var entry domain.AuditLog
		if err := rows.Scan(&entry.ID, &entry.StoreID, &entry.ActorUsername, &entry.ActorRole, &entry.Action, &entry.EntityType, &entry.EntityID, &entry.Detail, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.CreatedAt = entry.CreatedAt.UTC()
		logs = append(logs, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *Store) CreateShift(ctx context.Context, shift domain.Shift) (*domain.Shift, error) {
	if strings.TrimSpace(shift.StoreID) == "" || strings.TrimSpace(shift.TerminalID) == "" || strings.TrimSpace(shift.CashierName) == "" {
		return nil, store.ErrInvalidTransaction
	}
	if shift.ID == "" {
		shift.ID = xid.New("shift")
	}
	if shift.OpenedAt.IsZero() {
		shift.OpenedAt = time.Now().UTC()
	}
	shift.Status = domain.ShiftStatusOpen
	shift.ClosedAt = nil
	shift.ClosingCashCents = 0

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO shifts (
			id, store_id, terminal_id, cashier_name, opening_float_cents,
			closing_cash_cents, status, opened_at, closed_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, shift.ID, shift.StoreID, shift.TerminalID, shift.CashierName, shift.OpeningFloatCents,
		shift.ClosingCashCents, shift.Status, shift.OpenedAt, nullTime(shift.ClosedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, store.ErrInvalidTransaction
		}
		return nil, err
	}
	saved := shift
	return &saved, nil
}

func (s *Store) CloseActiveShift(ctx context.Context, storeID string, terminalID string, closingCashCents int64, closedAt time.Time) (*domain.Shift, error) {
	if strings.TrimSpace(storeID) == "" || strings.TrimSpace(terminalID) == "" {
		return nil, store.ErrInvalidTransaction
	}
	if closedAt.IsZero() {
		closedAt = time.Now().UTC()
	}

	var shift domain.Shift
	var closedAtNull sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		UPDATE shifts
		SET status = 'closed', closing_cash_cents = $3, closed_at = $4
		WHERE store_id = $1 AND terminal_id = $2 AND status = 'open'
		RETURNING id, store_id, terminal_id, cashier_name, opening_float_cents,
			closing_cash_cents, status, opened_at, closed_at
	`, storeID, terminalID, closingCashCents, closedAt).Scan(
		&shift.ID,
		&shift.StoreID,
		&shift.TerminalID,
		&shift.CashierName,
		&shift.OpeningFloatCents,
		&shift.ClosingCashCents,
		&shift.Status,
		&shift.OpenedAt,
		&closedAtNull,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	shift.OpenedAt = shift.OpenedAt.UTC()
	if closedAtNull.Valid {
		at := closedAtNull.Time.UTC()
		shift.ClosedAt = &at
	}
	return &shift, nil
}

func (s *Store) GetActiveShift(ctx context.Context, storeID string, terminalID string) (*domain.Shift, error) {
	var shift domain.Shift
	var closedAtNull sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, store_id, terminal_id, cashier_name, opening_float_cents,
			closing_cash_cents, status, opened_at, closed_at
		FROM shifts
		WHERE store_id = $1 AND terminal_id = $2 AND status = 'open'
		ORDER BY opened_at DESC
		LIMIT 1
	`, storeID, terminalID).Scan(
		&shift.ID,
		&shift.StoreID,
		&shift.TerminalID,
		&shift.CashierName,
		&shift.OpeningFloatCents,
		&shift.ClosingCashCents,
		&shift.Status,
		&shift.OpenedAt,
		&closedAtNull,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	shift.OpenedAt = shift.OpenedAt.UTC()
	if closedAtNull.Valid {
		at := closedAtNull.Time.UTC()
		shift.ClosedAt = &at
	}
	return &shift, nil
}

func (s *Store) CreatePromo(ctx context.Context, promo domain.PromoRule) (*domain.PromoRule, error) {
	promo.Name = strings.TrimSpace(promo.Name)
	if promo.Name == "" {
		return nil, store.ErrInvalidTransaction
	}
	if promo.Type != "cart_percent" && promo.Type != "flat_cart" {
		return nil, store.ErrInvalidTransaction
	}
	if promo.Type == "cart_percent" && promo.DiscountPercent <= 0 {
		return nil, store.ErrInvalidTransaction
	}
	if promo.Type == "flat_cart" && promo.FlatDiscountCents <= 0 {
		return nil, store.ErrInvalidTransaction
	}
	if promo.ID == "" {
		promo.ID = xid.New("promo")
	}
	if promo.CreatedAt.IsZero() {
		promo.CreatedAt = time.Now().UTC()
	}
	promo.Active = true

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO promo_rules (
			id, name, type, min_subtotal_cents, discount_percent, flat_discount_cents, active, created_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
	`, promo.ID, promo.Name, promo.Type, promo.MinSubtotalCents, promo.DiscountPercent, promo.FlatDiscountCents, promo.Active, promo.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, store.ErrInvalidTransaction
		}
		return nil, err
	}
	saved := promo
	return &saved, nil
}

func (s *Store) ListPromos(ctx context.Context) ([]domain.PromoRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, min_subtotal_cents, discount_percent, flat_discount_cents, active, created_at
		FROM promo_rules
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	promos := make([]domain.PromoRule, 0, 16)
	for rows.Next() {
		var promo domain.PromoRule
		if err := rows.Scan(&promo.ID, &promo.Name, &promo.Type, &promo.MinSubtotalCents, &promo.DiscountPercent, &promo.FlatDiscountCents, &promo.Active, &promo.CreatedAt); err != nil {
			return nil, err
		}
		promo.CreatedAt = promo.CreatedAt.UTC()
		promos = append(promos, promo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return promos, nil
}

func (s *Store) UpdatePromoActive(ctx context.Context, promoID string, active bool) (*domain.PromoRule, error) {
	var promo domain.PromoRule
	err := s.db.QueryRowContext(ctx, `
		UPDATE promo_rules
		SET active = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, name, type, min_subtotal_cents, discount_percent, flat_discount_cents, active, created_at
	`, promoID, active).Scan(
		&promo.ID,
		&promo.Name,
		&promo.Type,
		&promo.MinSubtotalCents,
		&promo.DiscountPercent,
		&promo.FlatDiscountCents,
		&promo.Active,
		&promo.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	promo.CreatedAt = promo.CreatedAt.UTC()
	return &promo, nil
}

func (s *Store) RebuildAssociationPairs(ctx context.Context, storeID string) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ti.transaction_id, ti.sku
		FROM transaction_items ti
		JOIN transactions t ON t.id = ti.transaction_id
		WHERE t.store_id = $1 AND t.status = $2
	`, storeID, domain.TxStatusPaid)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	txToSkus := map[string]map[string]struct{}{}
	for rows.Next() {
		var txID string
		var sku string
		if err := rows.Scan(&txID, &sku); err != nil {
			return 0, err
		}
		bucket := txToSkus[txID]
		if bucket == nil {
			bucket = map[string]struct{}{}
			txToSkus[txID] = bucket
		}
		bucket[sku] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	sourceCount := map[string]int{}
	pairCount := map[string]int{}
	for _, skuSet := range txToSkus {
		skus := make([]string, 0, len(skuSet))
		for sku := range skuSet {
			skus = append(skus, sku)
			sourceCount[sku]++
		}
		for _, source := range skus {
			for _, target := range skus {
				if source == target {
					continue
				}
				pairCount[source+"->"+target]++
			}
		}
	}

	type computedPair struct {
		source   string
		target   string
		affinity float64
	}
	computed := make([]computedPair, 0, len(pairCount))
	for key, cnt := range pairCount {
		arrow := -1
		for i := 0; i+1 < len(key); i++ {
			if key[i] == '-' && key[i+1] == '>' {
				arrow = i
				break
			}
		}
		if arrow < 1 {
			continue
		}
		source := key[:arrow]
		target := key[arrow+2:]
		srcCount := sourceCount[source]
		if srcCount < 1 {
			continue
		}
		affinity := float64(cnt) / float64(srcCount)
		if affinity < 0.2 {
			continue
		}
		computed = append(computed, computedPair{source: source, target: target, affinity: affinity})
	}

	sort.Slice(computed, func(i, j int) bool {
		if computed[i].source == computed[j].source {
			if computed[i].affinity == computed[j].affinity {
				return computed[i].target < computed[j].target
			}
			return computed[i].affinity > computed[j].affinity
		}
		return computed[i].source < computed[j].source
	})
	if len(computed) > 300 {
		computed = computed[:300]
	}

	pgTx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer func() { _ = pgTx.Rollback() }()

	_, err = pgTx.ExecContext(ctx, `DELETE FROM association_item_pairs`)
	if err != nil {
		return 0, err
	}

	for _, pair := range computed {
		_, err := pgTx.ExecContext(ctx, `
			INSERT INTO association_item_pairs (source_sku, target_sku, support, confidence, lift, affinity_score, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,now())
		`, pair.source, pair.target, 0.0, pair.affinity, 0.0, pair.affinity)
		if err != nil {
			return 0, err
		}
	}

	if err := pgTx.Commit(); err != nil {
		return 0, err
	}

	return len(computed), nil
}

func (s *Store) CreateHeldCart(ctx context.Context, held domain.HeldCart) (*domain.HeldCart, error) {
	if held.ID == "" {
		held.ID = xid.New("hold")
	}
	if held.HeldAt.IsZero() {
		held.HeldAt = time.Now().UTC()
	}
	if held.StoreID == "" || held.TerminalID == "" || len(held.CartItems) == 0 {
		return nil, store.ErrInvalidTransaction
	}

	itemsJSON, err := json.Marshal(held.CartItems)
	if err != nil {
		return nil, err
	}
	splitsJSON, err := json.Marshal(held.PaymentSplits)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO held_carts (
			id, store_id, terminal_id, cashier_username, note, cart_items,
			discount_cents, tax_rate_percent, payment_method, payment_reference,
			payment_splits, cash_received_cents, manual_override, held_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, held.ID, held.StoreID, held.TerminalID, held.CashierUsername, held.Note, itemsJSON,
		held.DiscountCents, held.TaxRatePercent, held.PaymentMethod, nullIfEmpty(held.PaymentReference),
		splitsJSON, held.CashReceivedCents, held.ManualOverride, held.HeldAt)
	if err != nil {
		return nil, err
	}
	saved := held
	return &saved, nil
}

func (s *Store) ListHeldCarts(ctx context.Context, storeID string, terminalID string, limit int) ([]domain.HeldCart, error) {
	if limit < 1 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, store_id, terminal_id, cashier_username, note, cart_items,
			discount_cents, tax_rate_percent, payment_method, payment_reference,
			payment_splits, cash_received_cents, manual_override, held_at
		FROM held_carts
		WHERE store_id = $1 AND terminal_id = $2
		ORDER BY held_at DESC
		LIMIT $3
	`, storeID, terminalID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	helds := make([]domain.HeldCart, 0, limit)
	for rows.Next() {
		var held domain.HeldCart
		var itemsRaw []byte
		var splitsRaw []byte
		var paymentReference sql.NullString
		if err := rows.Scan(
			&held.ID,
			&held.StoreID,
			&held.TerminalID,
			&held.CashierUsername,
			&held.Note,
			&itemsRaw,
			&held.DiscountCents,
			&held.TaxRatePercent,
			&held.PaymentMethod,
			&paymentReference,
			&splitsRaw,
			&held.CashReceivedCents,
			&held.ManualOverride,
			&held.HeldAt,
		); err != nil {
			return nil, err
		}
		held.HeldAt = held.HeldAt.UTC()
		if paymentReference.Valid {
			held.PaymentReference = paymentReference.String
		}
		if len(itemsRaw) > 0 {
			if err := json.Unmarshal(itemsRaw, &held.CartItems); err != nil {
				return nil, err
			}
		}
		if len(splitsRaw) > 0 {
			_ = json.Unmarshal(splitsRaw, &held.PaymentSplits)
		}
		helds = append(helds, held)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return helds, nil
}

func (s *Store) PopHeldCart(ctx context.Context, holdID string) (*domain.HeldCart, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var held domain.HeldCart
	var itemsRaw []byte
	var splitsRaw []byte
	var paymentReference sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT id, store_id, terminal_id, cashier_username, note, cart_items,
			discount_cents, tax_rate_percent, payment_method, payment_reference,
			payment_splits, cash_received_cents, manual_override, held_at
		FROM held_carts
		WHERE id = $1
		FOR UPDATE
	`, holdID).Scan(
		&held.ID,
		&held.StoreID,
		&held.TerminalID,
		&held.CashierUsername,
		&held.Note,
		&itemsRaw,
		&held.DiscountCents,
		&held.TaxRatePercent,
		&held.PaymentMethod,
		&paymentReference,
		&splitsRaw,
		&held.CashReceivedCents,
		&held.ManualOverride,
		&held.HeldAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	if paymentReference.Valid {
		held.PaymentReference = paymentReference.String
	}
	if len(itemsRaw) > 0 {
		if err := json.Unmarshal(itemsRaw, &held.CartItems); err != nil {
			return nil, err
		}
	}
	if len(splitsRaw) > 0 {
		_ = json.Unmarshal(splitsRaw, &held.PaymentSplits)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM held_carts WHERE id = $1`, holdID)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, store.ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &held, nil
}

func (s *Store) DeleteHeldCart(ctx context.Context, holdID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM held_carts WHERE id = $1`, holdID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) CreateSupplier(ctx context.Context, supplier domain.Supplier) (*domain.Supplier, error) {
	supplier.Name = strings.TrimSpace(supplier.Name)
	supplier.Phone = strings.TrimSpace(supplier.Phone)
	if supplier.Name == "" {
		return nil, store.ErrInvalidTransaction
	}
	if supplier.ID == "" {
		supplier.ID = xid.New("sup")
	}
	if supplier.CreatedAt.IsZero() {
		supplier.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO suppliers (id, name, phone, created_at)
		VALUES ($1,$2,$3,$4)
	`, supplier.ID, supplier.Name, nullIfEmpty(supplier.Phone), supplier.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, store.ErrInvalidTransaction
		}
		return nil, err
	}
	saved := supplier
	return &saved, nil
}

func (s *Store) ListSuppliers(ctx context.Context) ([]domain.Supplier, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(phone,''), created_at
		FROM suppliers
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	suppliers := make([]domain.Supplier, 0, 64)
	for rows.Next() {
		var item domain.Supplier
		if err := rows.Scan(&item.ID, &item.Name, &item.Phone, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = item.CreatedAt.UTC()
		suppliers = append(suppliers, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return suppliers, nil
}

func (s *Store) CreatePurchaseOrder(ctx context.Context, po domain.PurchaseOrder) (*domain.PurchaseOrder, error) {
	if po.ID == "" {
		po.ID = xid.New("po")
	}
	if po.CreatedAt.IsZero() {
		po.CreatedAt = time.Now().UTC()
	}
	if po.Status == "" {
		po.Status = "draft"
	}
	if po.StoreID == "" || po.SupplierID == "" || len(po.Items) == 0 {
		return nil, store.ErrInvalidTransaction
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO purchase_orders (id, store_id, supplier_id, status, created_at)
		VALUES ($1,$2,$3,$4,$5)
	`, po.ID, po.StoreID, po.SupplierID, po.Status, po.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	items := make([]domain.PurchaseOrderItem, 0, len(po.Items))
	for _, item := range po.Items {
		item.SKU = strings.ToUpper(strings.TrimSpace(item.SKU))
		if item.SKU == "" || item.Qty < 1 || item.CostCents < 1 {
			return nil, store.ErrInvalidTransaction
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_order_items (purchase_order_id, sku, qty, cost_cents)
			VALUES ($1,$2,$3,$4)
		`, po.ID, item.SKU, item.Qty, item.CostCents)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return nil, store.ErrNotFound
			}
			return nil, err
		}
		items = append(items, item)
	}
	po.Items = items

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	saved := po
	return &saved, nil
}

func (s *Store) GetPurchaseOrderByID(ctx context.Context, purchaseOrderID string) (*domain.PurchaseOrder, error) {
	var po domain.PurchaseOrder
	var receivedAt sql.NullTime
	var receivedBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, store_id, supplier_id, status, created_at, received_at, received_by
		FROM purchase_orders
		WHERE id = $1
	`, purchaseOrderID).Scan(
		&po.ID,
		&po.StoreID,
		&po.SupplierID,
		&po.Status,
		&po.CreatedAt,
		&receivedAt,
		&receivedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	po.CreatedAt = po.CreatedAt.UTC()
	if receivedAt.Valid {
		at := receivedAt.Time.UTC()
		po.ReceivedAt = &at
	}
	if receivedBy.Valid {
		po.ReceivedBy = receivedBy.String
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT sku, qty, cost_cents
		FROM purchase_order_items
		WHERE purchase_order_id = $1
		ORDER BY id ASC
	`, po.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.PurchaseOrderItem, 0, 8)
	for rows.Next() {
		var item domain.PurchaseOrderItem
		if err := rows.Scan(&item.SKU, &item.Qty, &item.CostCents); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	po.Items = items
	return &po, nil
}

func (s *Store) ListPurchaseOrders(ctx context.Context, storeID string, status string, limit int) ([]domain.PurchaseOrder, error) {
	if limit < 1 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, store_id, supplier_id, status, created_at, received_at, received_by
		FROM purchase_orders
		WHERE ($1 = '' OR store_id = $1)
			AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3
	`, storeID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.PurchaseOrder, 0, limit)
	ids := make([]string, 0, limit)
	for rows.Next() {
		var po domain.PurchaseOrder
		var receivedAt sql.NullTime
		var receivedBy sql.NullString
		if err := rows.Scan(&po.ID, &po.StoreID, &po.SupplierID, &po.Status, &po.CreatedAt, &receivedAt, &receivedBy); err != nil {
			return nil, err
		}
		po.CreatedAt = po.CreatedAt.UTC()
		if receivedAt.Valid {
			at := receivedAt.Time.UTC()
			po.ReceivedAt = &at
		}
		if receivedBy.Valid {
			po.ReceivedBy = receivedBy.String
		}
		result = append(result, po)
		ids = append(ids, po.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return result, nil
	}

	itemRows, err := s.db.QueryContext(ctx, `
		SELECT purchase_order_id, sku, qty, cost_cents
		FROM purchase_order_items
		WHERE purchase_order_id = ANY($1)
		ORDER BY id ASC
	`, ids)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()

	itemMap := make(map[string][]domain.PurchaseOrderItem, len(ids))
	for itemRows.Next() {
		var poID string
		var item domain.PurchaseOrderItem
		if err := itemRows.Scan(&poID, &item.SKU, &item.Qty, &item.CostCents); err != nil {
			return nil, err
		}
		itemMap[poID] = append(itemMap[poID], item)
	}
	if err := itemRows.Err(); err != nil {
		return nil, err
	}

	for i := range result {
		result[i].Items = itemMap[result[i].ID]
	}
	return result, nil
}

func (s *Store) ReceivePurchaseOrder(ctx context.Context, purchaseOrderID string, receivedBy string, receivedAt time.Time) (*domain.PurchaseOrder, error) {
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	receivedBy = strings.TrimSpace(receivedBy)
	if receivedBy == "" {
		receivedBy = "system"
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var po domain.PurchaseOrder
	var currentReceivedAt sql.NullTime
	var currentReceivedBy sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT id, store_id, supplier_id, status, created_at, received_at, received_by
		FROM purchase_orders
		WHERE id = $1
		FOR UPDATE
	`, purchaseOrderID).Scan(
		&po.ID,
		&po.StoreID,
		&po.SupplierID,
		&po.Status,
		&po.CreatedAt,
		&currentReceivedAt,
		&currentReceivedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	po.CreatedAt = po.CreatedAt.UTC()
	if po.Status == "received" || po.Status == "cancelled" {
		return nil, store.ErrInvalidTransaction
	}

	itemRows, err := tx.QueryContext(ctx, `
		SELECT sku, qty, cost_cents
		FROM purchase_order_items
		WHERE purchase_order_id = $1
		ORDER BY id ASC
	`, purchaseOrderID)
	if err != nil {
		return nil, err
	}
	items := make([]domain.PurchaseOrderItem, 0, 8)
	for itemRows.Next() {
		var item domain.PurchaseOrderItem
		if err := itemRows.Scan(&item.SKU, &item.Qty, &item.CostCents); err != nil {
			_ = itemRows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := itemRows.Err(); err != nil {
		_ = itemRows.Close()
		return nil, err
	}
	_ = itemRows.Close()
	if len(items) == 0 {
		return nil, store.ErrInvalidTransaction
	}
	po.Items = items

	skus := make([]string, 0, len(items))
	for _, item := range items {
		skus = append(skus, item.SKU)
	}

	stockRows, err := tx.QueryContext(ctx, `
		SELECT sku, qty
		FROM inventory_stocks
		WHERE store_id = $1 AND sku = ANY($2)
		FOR UPDATE
	`, po.StoreID, skus)
	if err != nil {
		return nil, err
	}
	stockMap := make(map[string]int, len(skus))
	for stockRows.Next() {
		var sku string
		var qty int
		if err := stockRows.Scan(&sku, &qty); err != nil {
			_ = stockRows.Close()
			return nil, err
		}
		stockMap[sku] = qty
	}
	if err := stockRows.Err(); err != nil {
		_ = stockRows.Close()
		return nil, err
	}
	_ = stockRows.Close()

	costRows, err := tx.QueryContext(ctx, `
		SELECT sku, cost_cents
		FROM product_costs
		WHERE store_id = $1 AND sku = ANY($2)
		FOR UPDATE
	`, po.StoreID, skus)
	if err != nil {
		return nil, err
	}
	costMap := make(map[string]int64, len(skus))
	for costRows.Next() {
		var sku string
		var cost int64
		if err := costRows.Scan(&sku, &cost); err != nil {
			_ = costRows.Close()
			return nil, err
		}
		costMap[sku] = cost
	}
	if err := costRows.Err(); err != nil {
		_ = costRows.Close()
		return nil, err
	}
	_ = costRows.Close()

	for idx, item := range items {
		currentQty := stockMap[item.SKU]
		prevCost := costMap[item.SKU]
		if prevCost < 1 {
			prevCost = item.CostCents
		}
		newCost := weightedCostCents(prevCost, currentQty, item.CostCents, item.Qty)

		_, err = tx.ExecContext(ctx, `
			INSERT INTO inventory_stocks (store_id, sku, qty, updated_at)
			VALUES ($1,$2,$3,now())
			ON CONFLICT (store_id, sku)
			DO UPDATE SET qty = inventory_stocks.qty + EXCLUDED.qty, updated_at = now()
		`, po.StoreID, item.SKU, item.Qty)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO product_costs (store_id, sku, cost_cents, updated_at)
			VALUES ($1,$2,$3,now())
			ON CONFLICT (store_id, sku)
			DO UPDATE SET cost_cents = EXCLUDED.cost_cents, updated_at = now()
		`, po.StoreID, item.SKU, newCost)
		if err != nil {
			return nil, err
		}
		lotCode := fmt.Sprintf("PO-%s-%02d", purchaseOrderID, idx+1)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO inventory_lots (
				id, store_id, sku, lot_code, expiry_date, qty_received, qty_available,
				cost_cents, source_type, source_id, notes, received_at, updated_at
			)
			VALUES ($1,$2,$3,$4,NULL,$5,$6,$7,'purchase_order',$8,$9,$10,now())
		`, xid.New("lot"), po.StoreID, item.SKU, lotCode, item.Qty, item.Qty, item.CostCents, purchaseOrderID, "auto lot from purchase order receive", receivedAt)
		if err != nil {
			return nil, err
		}
		stockMap[item.SKU] = currentQty + item.Qty
		costMap[item.SKU] = newCost
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE purchase_orders
		SET status = 'received', received_at = $2, received_by = $3
		WHERE id = $1 AND status <> 'received'
	`, purchaseOrderID, receivedAt, receivedBy)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, store.ErrInvalidTransaction
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO purchase_order_receipts (id, purchase_order_id, received_by, created_at)
		VALUES ($1,$2,$3,$4)
	`, xid.New("por"), purchaseOrderID, receivedBy, receivedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	po.Status = "received"
	po.ReceivedBy = receivedBy
	po.ReceivedAt = &receivedAt
	return &po, nil
}

func (s *Store) GetProductCosts(ctx context.Context, storeID string, skus []string) (map[string]int64, error) {
	result := make(map[string]int64, len(skus))
	if len(skus) == 0 {
		return result, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT sku, cost_cents
		FROM product_costs
		WHERE store_id = $1 AND sku = ANY($2)
	`, storeID, skus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sku string
		var cost int64
		if err := rows.Scan(&sku, &cost); err != nil {
			return nil, err
		}
		result[sku] = cost
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, sku := range skus {
		if _, ok := result[sku]; !ok {
			result[sku] = 0
		}
	}
	return result, nil
}

func (s *Store) UpsertProductCost(ctx context.Context, storeID string, sku string, costCents int64) error {
	if sku == "" || costCents < 1 {
		return store.ErrInvalidTransaction
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO product_costs (store_id, sku, cost_cents, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (store_id, sku)
		DO UPDATE SET cost_cents = EXCLUDED.cost_cents, updated_at = now()
	`, storeID, sku, costCents)
	return err
}

func (s *Store) CreateUser(ctx context.Context, user domain.UserAccount) error {
	user.Username = strings.ToLower(strings.TrimSpace(user.Username))
	if user.Username == "" || strings.TrimSpace(user.Password) == "" {
		return store.ErrInvalidTransaction
	}
	if user.Role == "" {
		user.Role = "cashier"
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_users (username, password, role, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,now())
	`, user.Username, user.Password, user.Role, user.Active, user.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return store.ErrInvalidTransaction
		}
		return err
	}
	return nil
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.UserAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT username, password, role, active, created_at
		FROM app_users
		ORDER BY username ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.UserAccount, 0, 16)
	for rows.Next() {
		var user domain.UserAccount
		if err := rows.Scan(&user.Username, &user.Password, &user.Role, &user.Active, &user.CreatedAt); err != nil {
			return nil, err
		}
		user.CreatedAt = user.CreatedAt.UTC()
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Store) UpdateUserPassword(ctx context.Context, username string, password string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || strings.TrimSpace(password) == "" {
		return store.ErrInvalidTransaction
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE app_users
		SET password = $2, updated_at = now()
		WHERE username = $1
	`, username, password)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func weightedCostCents(oldCost int64, oldQty int, incomingCost int64, incomingQty int) int64 {
	if incomingQty <= 0 || incomingCost <= 0 {
		return oldCost
	}
	if oldQty <= 0 || oldCost <= 0 {
		return incomingCost
	}
	totalQty := oldQty + incomingQty
	if totalQty <= 0 {
		return incomingCost
	}
	totalValue := oldCost*int64(oldQty) + incomingCost*int64(incomingQty)
	weighted := int64(math.Round(float64(totalValue) / float64(totalQty)))
	if weighted < 1 {
		return 1
	}
	return weighted
}

func uniqueSKUs(items []domain.TransactionLine) []string {
	if len(items) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.SKU == "" {
			continue
		}
		set[item.SKU] = struct{}{}
	}

	skus := make([]string, 0, len(set))
	for sku := range set {
		skus = append(skus, sku)
	}
	sort.Strings(skus)
	return skus
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func nowDateUTC(t time.Time) time.Time {
	return time.Date(t.UTC().Year(), t.UTC().Month(), t.UTC().Day(), 0, 0, 0, 0, time.UTC)
}

func nullIfEmpty(val string) any {
	if val == "" {
		return nil
	}
	return val
}

func nullDate(val *time.Time) any {
	if val == nil {
		return nil
	}
	return nowDateUTC(*val)
}

func nullTime(val *time.Time) any {
	if val == nil {
		return nil
	}
	return *val
}
