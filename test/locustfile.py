from locust import HttpUser, task, between
import os


class AuthUser(HttpUser):
    wait_time = between(1, 3)

    # Sample resource IDs — override per environment via env vars.
    # NOTE (post Mongo->Postgres migration): journal ids are 24-char hex strings
    # (converted from the old Mongo ObjectIDs); branch/supplier/transaction/product
    # ids are UUID strings.
    branch_id = os.getenv("BRANCH_ID", "2dbbbb13-60be-425e-b07c-fa4889f0400d")
    journal_id = os.getenv("JOURNAL_ID", "6a5236a319076a3c306ed8c5")
    supplier_id = os.getenv("SUPPLIER_ID", "f5602699-8487-4a44-8ef2-eec9815dfaf8")
    transaction_id = os.getenv("TRANSACTION_ID", "bfb09657-5e96-453f-9608-b46381ac2037")

    # host = os.getenv("HOST")
    # if not host:
    #     print("HOST is not set")
    #     exit(1)

    def on_start(self):
        """Log in and store the access token for authenticated requests."""
        payload = {"username": "aslon", "password": "aslon"}
        self.refresh_token = None

        # Login is public. It now returns an access + refresh token pair
        # (TokenPairResponse) — not {"data": token} as before the migration.
        with self.client.post(
            "/api/v1/admin/auth/login", json=payload, catch_response=True
        ) as response:
            if response.status_code == 200:
                data = response.json()
                token = data.get("access_token")
                self.refresh_token = data.get("refresh_token")
                if token:
                    # The PASETO guard expects the raw token (no "Bearer" prefix).
                    self.client.headers.update(
                        {
                            "Authorization": f"{token}",
                            "Content-Type": "application/json",
                        }
                    )
                else:
                    response.failure("No access_token in response")
            else:
                response.failure(f"Login failed: {response.status_code}")

    @task
    def refresh_token_pair(self):
        """Exercise the refresh endpoint and rotate the stored token pair."""
        if not self.refresh_token:
            return
        with self.client.post(
            "/api/v1/admin/auth/refresh",
            json={"refresh_token": self.refresh_token},
            catch_response=True,
        ) as resp:
            if resp.status_code == 200:
                data = resp.json()
                access = data.get("access_token")
                # keep the rotated refresh token for the next call
                self.refresh_token = data.get("refresh_token", self.refresh_token)
                if access:
                    self.client.headers.update({"Authorization": f"{access}"})
            else:
                resp.failure(f"Refresh failed: {resp.status_code}")

    @task
    def get_journals(self):
        """Authenticated GET request"""
        resp = self.client.get(f"/api/v1/admin/journals/branch/{self.branch_id}")
        if resp.status_code == 400:
            print(resp.text)

    @task
    def get_journal(self):
        resp = self.client.get(f"/api/v1/admin/journals/{self.journal_id}")
        if resp.status_code == 400:
            print(resp.text)

    @task
    def get_all_suppliers(self):
        resp = self.client.get("/api/v1/admin/suppliers")
        if resp.status_code == 400:
            print(resp.text)

    @task
    def get_supplier(self):
        resp = self.client.get(f"/api/v1/admin/suppliers/{self.supplier_id}")
        if resp.status_code == 400:
            print(resp.text)

    @task
    def get_transactions(self):
        resp = self.client.get(f"/api/v1/admin/transactions/branch/{self.branch_id}")
        if resp.status_code == 400:
            print(resp.text)

    @task
    def get_transaction(self):
        resp = self.client.get(f"/api/v1/admin/transactions/{self.transaction_id}")
        if resp.status_code == 400:
            print(resp.text)

    @task
    def query_products(self):
        resp = self.client.get("/api/v1/admin/products")
        if resp.status_code == 400:
            print(resp.text)
