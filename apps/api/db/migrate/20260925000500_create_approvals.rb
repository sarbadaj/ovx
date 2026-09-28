class CreateApprovals < ActiveRecord::Migration[8.0]
  def change
    create_table :approvals do |t|
      t.references :task, null: false, foreign_key: true
      t.references :tool_call, null: false, foreign_key: true, index: { unique: true }
      t.string :status, null: false, default: "pending"
      t.text :reason, null: false
      t.jsonb :requested_capabilities, null: false, default: []
      t.datetime :decided_at
      t.timestamps
    end
    add_index :approvals, :status
  end
end
