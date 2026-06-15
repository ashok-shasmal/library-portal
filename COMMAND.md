# Library Portal curl Commands

All commands assume the app is running at `http://localhost:8080`.

## 1) Register a normal user

```bash
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Normal User","email":"user@example.com","password":"userpass"}'
```

## 2) Register an admin user

```bash
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Admin User","email":"admin@example.com","password":"adminpass","role":"ADMIN"}'
```

## 3) Login as a user

```bash
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"userpass"}'
```

## 4) Login as an admin

```bash
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"adminpass"}'
```

## 5) Add a book (admin only)

```bash
curl -X POST http://localhost:8080/books \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"title":"The Go Programming Language","author_name":"Alan A. A. Donovan"}'
```

## 6) Add a borrow record as a normal user

```bash
curl -X POST http://localhost:8080/borrow_records \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -d '{"book_id": 123}'
```

## 7) Add a borrow record as admin for a specific user

```bash
curl -X POST http://localhost:8080/borrow_records \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"user_id": 45, "book_id": 123}'
```

## Notes

- The `/register` endpoint accepts an optional `role` field.
- Use `role: "ADMIN"` to create an admin account.
- Normal registration without `role` defaults to `USER`.
