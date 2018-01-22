# Tests

## Add
- Add one user, and see it if gets inserted into DB
	- Add to same user, see if balance is updated	

## Quote

- Check that quoteserver returns stock price for given test stock

## Buy

- Buy stock "S" for user "A" with amount "M".
- Check that users balance is reduced
- Check that reservations have been made for stock "S"
- Check that timeout works, and money is returned

## Commit Buy
- Executes **most recent** Buy command
	- Check that stock is updated
