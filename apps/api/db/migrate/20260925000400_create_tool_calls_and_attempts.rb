class CreateToolCallsAndAttempts < ActiveRecord::Migration[8.0]
  def change
    create_table :tool_calls do |t|
      t.references :task, null: false, foreign_key: true
      t.string :tool_name, null: false
      t.jsonb :arguments, null: false, default: {}
      t.string :input_hash
      t.jsonb :required_capabilities, null: false, default: []
      t.string :state, null: false, default: "requested"
      t.datetime :started_at
      t.datetime :completed_at
      t.timestamps
    end
    add_index :tool_calls, [:task_id, :state]

    create_table :tool_attempts do |t|
      t.references :tool_call, null: false, foreign_key: true
      t.integer :attempt_number, null: false, default: 1
      t.string :state, null: false
      t.datetime :started_at
      t.datetime :finished_at
      t.integer :exit_code
      t.jsonb :result, null: false, default: {}
      t.text :error_message
      t.timestamps
    end
    add_index :tool_attempts, [:tool_call_id, :attempt_number], unique: true
  end
end
