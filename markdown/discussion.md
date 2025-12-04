i need you to discuss this project. this project is going to be a ledger for a transaction from a payment gateway DOKU

there is a transaction, a transaction is made from payment that are pending, success or failed. each transaction has amount, date, invoice number, payment method, and status. transaction mean an income for the ledger.

there is a settlement, settlemenet is a transaction that has been settled from the payment gateway with a deduction of fee. so if the transaction is IDR50000 it will be recorded as IDR45000 as example, 5000 is the fee deducted by the payment gateway. settlement has nomor batch, settlement date, real settelement date, currency, settelement amounmt,  bank name, bank account number, type(account or sub-account), and status(In Progress	, Transferred).

i do add ledger_account_bank to store an information of user bank account to receive their income.
this project will help users to keep track of their transactions and settlements from the DOKU payment gateway. by maintaining a ledger, users can easily monitor their income, fees deducted, and the status of their transactions and settlements.

i do add ledger_balance, i think the best way to refactoring it name to be ledger_wallet, it will act as a wallet, everytime there is a successful transaction, the amount will be added to the wallet, and when there is a settlement, the amount will be deducted from the wallet. this way users can have a clear view of their available balance after considering both income from transactions and deductions from settlements.
