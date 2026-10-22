-- The five customers who bought the most, discounts included.
SELECT c.company_name AS customer,
       count(DISTINCT o.id) AS orders,
       round(sum(d.unit_price * d.quantity * (1 - d.discount)), 2) AS revenue
FROM customer c
JOIN "order" o ON o.customer_id = c.id
JOIN order_detail d ON d.order_id = o.id
GROUP BY c.id
ORDER BY revenue DESC
LIMIT 5;
