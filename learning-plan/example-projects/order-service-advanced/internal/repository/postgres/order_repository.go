package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"orderservice/internal/domain"
)

// OrderRepository implements domain.OrderRepository with raw SQL via pgx.
// CreateOrder and SetOrderStatus run inside an explicit transaction and use
// SELECT ... FOR UPDATE row locks so concurrent requests cannot oversell.
type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

func (r *OrderRepository) CreateOrder(ctx context.Context, lines []domain.OrderLine) (*domain.Order, error) {
	// Lock product rows in a deterministic order to avoid deadlocks between
	// two checkouts that touch the same products.
	sort.Slice(lines, func(i, j int) bool { return lines[i].ProductID < lines[j].ProductID })

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) // no-op once the tx is committed

	var total int64
	items := make([]domain.OrderItem, 0, len(lines))
	for _, line := range lines {
		var (
			id    int64
			name  string
			price int64
			stock int
		)
		// FOR UPDATE blocks any other transaction from touching this product
		// row until we commit — the core of the no-oversell guarantee.
		err := tx.QueryRow(ctx,
			`SELECT id, name, price_cents, stock FROM products WHERE id = $1 FOR UPDATE`,
			line.ProductID,
		).Scan(&id, &name, &price, &stock)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("lock product %d: %w", line.ProductID, err)
		}
		if stock < line.Quantity {
			return nil, domain.InsufficientStockError{ProductID: id, Available: stock, Requested: line.Quantity}
		}
		if _, err := tx.Exec(ctx,
			`UPDATE products SET stock = stock - $1, updated_at = now() WHERE id = $2`,
			line.Quantity, id,
		); err != nil {
			return nil, fmt.Errorf("decrement stock: %w", err)
		}
		total += price * int64(line.Quantity)
		items = append(items, domain.OrderItem{
			ProductID:      id,
			ProductName:    name,
			Quantity:       line.Quantity,
			UnitPriceCents: price,
		})
	}

	var (
		orderID   int64
		createdAt time.Time
		updatedAt time.Time
	)
	if err := tx.QueryRow(ctx,
		`INSERT INTO orders (status, total_cents) VALUES ($1, $2) RETURNING id, created_at, updated_at`,
		string(domain.StatusPending), total,
	).Scan(&orderID, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("insert order: %w", err)
	}

	for i := range items {
		items[i].OrderID = orderID
		if err := tx.QueryRow(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price_cents)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			orderID, items[i].ProductID, items[i].ProductName, items[i].Quantity, items[i].UnitPriceCents,
		).Scan(&items[i].ID); err != nil {
			return nil, fmt.Errorf("insert order item: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &domain.Order{
		ID:         orderID,
		Status:     domain.StatusPending,
		TotalCents: total,
		Items:      items,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

func (r *OrderRepository) SetOrderStatus(ctx context.Context, id int64, to domain.OrderStatus) (*domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the order row, then enforce the state machine atomically.
	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock order: %w", err)
	}
	from := domain.OrderStatus(current)
	if !domain.CanTransition(from, to) {
		return nil, domain.InvalidTransitionError{From: from, To: to}
	}

	if to == domain.StatusCancelled {
		if err := restockOrder(ctx, tx, id); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE orders SET status = $1, updated_at = now() WHERE id = $2`, string(to), id); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return r.GetOrder(ctx, id)
}

// restockOrder adds each item's quantity back to its product. It reads every
// item fully before issuing updates because pgx cannot run a new query on the
// same transaction while a result set is still open.
func restockOrder(ctx context.Context, tx pgx.Tx, orderID int64) error {
	type line struct {
		productID int64
		qty       int
	}
	rows, err := tx.Query(ctx, `SELECT product_id, quantity FROM order_items WHERE order_id = $1`, orderID)
	if err != nil {
		return fmt.Errorf("read items: %w", err)
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.productID, &l.qty); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	sort.Slice(lines, func(i, j int) bool { return lines[i].productID < lines[j].productID })
	for _, l := range lines {
		if _, err := tx.Exec(ctx,
			`UPDATE products SET stock = stock + $1, updated_at = now() WHERE id = $2`,
			l.qty, l.productID,
		); err != nil {
			return fmt.Errorf("restock product %d: %w", l.productID, err)
		}
	}
	return nil
}

func (r *OrderRepository) GetOrder(ctx context.Context, id int64) (*domain.Order, error) {
	var (
		o      domain.Order
		status string
	)
	err := r.pool.QueryRow(ctx,
		`SELECT id, status, total_cents, created_at, updated_at FROM orders WHERE id = $1`, id,
	).Scan(&o.ID, &status, &o.TotalCents, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	o.Status = domain.OrderStatus(status)

	rows, err := r.pool.Query(ctx,
		`SELECT id, order_id, product_id, product_name, quantity, unit_price_cents
		 FROM order_items WHERE order_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("get order items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it domain.OrderItem
		if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.ProductName, &it.Quantity, &it.UnitPriceCents); err != nil {
			return nil, err
		}
		o.Items = append(o.Items, it)
	}
	return &o, rows.Err()
}

func (r *OrderRepository) ListOrders(ctx context.Context) ([]domain.Order, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, status, total_cents, created_at, updated_at FROM orders ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var out []domain.Order
	for rows.Next() {
		var (
			o      domain.Order
			status string
		)
		if err := rows.Scan(&o.ID, &status, &o.TotalCents, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.Status = domain.OrderStatus(status)
		out = append(out, o)
	}
	return out, rows.Err()
}
