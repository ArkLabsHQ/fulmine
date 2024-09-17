# ark-node

ark-node is a node implementation for the Ark Network, providing a secure and efficient way to interact with the Ark ecosystem.

## Table of Contents

- [Getting Started](#getting-started)
- [Using the Binary](#using-the-binary)
- [API Overview](#api-overview)

## Getting Started

To get started with ark-node you need Go `1.23.1` or higher.

```bash
git clone https://github.com/ArkLabsHQ/ark-node.git
cd ark-node
go mod download
make run
```

Now navigate to http://localhost:7000/app/ to see the dashboard.

## Using the Binary

To use the binary, you can download the latest release from the [releases page](https://github.com/ArkLabsHQ/ark-node/releases).

## API Overview

## API Documentation

Here's a high-level overview of the main API endpoints, including examples using curl:

### Wallet Service

1. Generate Seed

   ```sh
   curl -X GET http://localhost:7000/v1/wallet/genseed
   ```

2. Create Wallet

   ```sh
   curl -X POST http://localhost:7000/v1/wallet/create \
        -H "Content-Type: application/json" \
        -d '{"mnemonic": "your mnemonic", "password": "your_password", "asp_url": "https://asp.example.com"}'
   ```

3. Unlock Wallet

   ```sh
   curl -X POST http://localhost:7000/v1/wallet/unlock \
        -H "Content-Type: application/json" \
        -d '{"password": "your_password"}'
   ```

4. Lock Wallet

   ```sh
   curl -X POST http://localhost:7000/v1/wallet/lock \
        -H "Content-Type: application/json" \
        -d '{"password": "your_password"}'
   ```

5. Change Password

   ```sh
   curl -X POST http://localhost:7000/v1/password/change \
        -H "Content-Type: application/json" \
        -d '{"current_password": "old_password", "new_password": "new_password"}'
   ```

6. Get Wallet Status
  
   ```sh
   curl -X GET http://localhost:7000/v1/wallet/status
   ```

### Service API

1. Get Address

   ```sh
   curl -X GET http://localhost:7000/v1/address
   ```

2. Get Balance

   ```sh
   curl -X GET http://localhost:7000/v1/balance
   ```

3. Send Transaction

   ```sh
   curl -X POST http://localhost:7000/v1/send \
        -H "Content-Type: application/json" \
        -d '{"address": "recipient_address", "amount": 1000000}'
   ```

4. Get Transaction History

   ```sh
   curl -X GET http://localhost:7000/v1/transactions
   ```

### Notification Service

1. Add Webhook

   ```sh
   curl -X POST http://localhost:7000/v1/notifications/webhook \
        -H "Content-Type: application/json" \
        -d '{"endpoint": "https://your-webhook.com/endpoint", "event_type": "WEBHOOK_EVENT_TYPE_ROUND", "secret": "your_secret"}'
   ```

2. Remove Webhook

   ```sh
   curl -X DELETE http://localhost:7000/v1/notifications/webhook/{webhook_id}
   ```

3. List Webhooks

   ```sh
   curl -X GET http://localhost:7000/v1/notifications/webhooks
   ```

Note: Replace `http://localhost:7000` with the appropriate host and port where your ark-node is running. Also, ensure to replace placeholder values (like `your_password`, `recipient_address`, etc.) with actual values when making requests.

For more detailed information about request and response structures, please refer to the proto files in the `api-spec/protobuf/ark_node/v1/` directory.
